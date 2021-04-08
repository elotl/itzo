// +build !darwin

/*
Copyright 2020 Elotl Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"syscall"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/helper"
	"github.com/elotl/itzo/pkg/unit"
	"github.com/elotl/itzo/pkg/util"
	"github.com/elotl/wsstream"
	"github.com/golang/glog"
	"github.com/prometheus/procfs"
	"github.com/kr/pty"
)

const (
	wsTTYControlChan = 4
)

func (s *Server) runExec(ws *wsstream.WSReadWriter, params api.ExecParams) {
	if len(params.Command) == 0 {
		glog.Errorf("No command specified for exec")
		writeWSErrorExitcode(ws, "No command specified")
		return
	}

	unitName, err := s.podController.GetUnitName(params.UnitName)
	if err != nil {
		glog.Errorf("Getting unit %s: %v", params.UnitName, err)
		writeWSErrorExitcode(ws, err.Error())
		return
	}

	command := params.Command

	var env []string

	// allow us to skip entering namespace for testing
	if !params.SkipNSEnter {
		unit, err := unit.OpenUnit(s.installRootdir, unitName)
		if err != nil {
			errmsg := fmt.Errorf("Error opening unit %s for exec: %v",
				unitName, err)
			glog.Errorf("%v", errmsg)
			writeWSErrorExitcode(ws, "%v\n", errmsg)
			return
		}
		userLookup, err := util.NewPasswdUserLookup(unit.GetRootfs())
		if err != nil {
			errmsg := fmt.Errorf(
				"Error creating user lookup in %s for exec: %v", unitName, err)
			glog.Errorf("%v", errmsg)
			writeWSErrorExitcode(ws, "%v\n", errmsg)
			return
		}
		uid, gid, _, homedir, err := unit.GetUser(userLookup)
		if err != nil {
			errmsg := fmt.Errorf("Error getting unit %s user for exec: %v",
				unitName, err)
			glog.Errorf("%v", errmsg)
			writeWSErrorExitcode(ws, "%v\n", errmsg)
			return
		}
		pid, exists := s.podController.GetPid(unitName)
		if !exists {
			glog.Errorf("Error getting pid for unit %s", unitName)
			writeWSErrorExitcode(ws, "Could not find running process for unit named %s\n", unitName)
			return
		}
		proc, err := procfs.NewProc(pid)
		if err != nil {
			glog.Errorf("cannot read pseudofilesystem /proc")
			writeWSErrorExitcode(ws, "Could not find process %d for unit named %s\n",
				pid, unitName)
			return
		}
		environ, err := proc.Environ()
		if err != nil {
			glog.Errorf("Error getting process for unit %s", unitName)
			writeWSErrorExitcode(ws, "Could not find process %d for unit named %s\n",
				pid, unitName)
			return
		}
		for k, v := range environ {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		env = helper.EnsureDefaultEnviron(env, params.PodName, homedir)
		nsenterCmd := []string{
			"/usr/bin/nsenter",
			"-t",
			strconv.Itoa(pid),
			"-p",
			"-u",
			"-m",
			"-n",
		}
		if uid != 0 || gid != 0 {
			userSpec := []string{
				"-S",
				fmt.Sprintf("%d", uid),
				"-G",
				fmt.Sprintf("%d", gid),
			}
			nsenterCmd = append(nsenterCmd, userSpec...)
		}
		command = append(nsenterCmd, command...)
	}

	glog.Infof("Exec command: %s", command[0])
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = env
	if params.TTY {
		err = s.runExecTTY(ws, cmd, params.Interactive)
	} else {
		err = s.runExecCmd(ws, cmd, params.Interactive)
	}
	if err != nil {
		glog.Errorf("Error running exec command %s: %v", command[0], err)
		writeWSErrorExitcode(ws, err.Error())
		return
	}
}

func (s *Server) runExecCmd(ws *wsstream.WSReadWriter, cmd *exec.Cmd, interactive bool) error {
	if interactive {
		wsStdinReader := ws.CreateReader(0)
		inPipe, err := cmd.StdinPipe()
		if err != nil {
			glog.Errorf("Error creating stdin pipe: %v", err)
			return err
		}
		go io.Copy(inPipe, wsStdinReader)
	}

	var wg sync.WaitGroup

	wsStdoutWriter := ws.CreateWriter(wsstream.StdoutChan)
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		glog.Errorf("Error creating stdout pipe: %v", err)
		return err
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(wsStdoutWriter, outPipe)
	}()

	wsStderrWriter := ws.CreateWriter(wsstream.StderrChan)
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		glog.Errorf("Error creating stderr pipe: %v", err)
		return err
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(wsStderrWriter, errPipe)
	}()

	err = cmd.Start()
	if err != nil {
		glog.Errorf("Error starting command %+v: %v", cmd, err)
		return err
	}

	go ws.RunDispatch()

	waitForFinished(ws, cmd, &wg)
	return nil
}

func (s *Server) runExecTTY(ws *wsstream.WSReadWriter, cmd *exec.Cmd, interactive bool) error {
	tty, err := pty.Start(cmd)
	if err != nil {
		glog.Errorf("Error starting pty for exec command %s: %v", cmd.Path, err)
		return err
	}
	defer tty.Close()
	if interactive {
		wsStdinReader := ws.CreateReader(0)
		go func() {
			io.Copy(tty, wsStdinReader)
		}()
	}

	// handle resize terminal messages
	termChanges := ws.CreateReader(wsTTYControlChan)
	go func() {
		for {
			buf := make([]byte, 32*1024)
			n, err := termChanges.Read(buf)
			if err != nil {
				if err != io.EOF {
					glog.Errorf("Error reading terminal changes")
				}
				return
			}
			var s pty.Winsize
			err = json.Unmarshal(buf[0:n], &s)
			if err != nil {
				glog.Warningf("error unmarshalling pty resize: %s", err)
				// should we send these errors back on stderr?
				return
			}
			if err := pty.Setsize(tty, &s); err != nil {
				glog.Warningf("error resizing pty: %s", err)
				return
			}
			glog.Infof("Exec PTY has been resized to %+v", s)
		}
	}()

	var wg sync.WaitGroup
	wsStdoutWriter := ws.CreateWriter(1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(wsStdoutWriter, tty)
	}()

	go ws.RunDispatch()
	waitForFinished(ws, cmd, &wg)
	return nil
}

func waitForFinished(ws *wsstream.WSReadWriter, cmd *exec.Cmd, wg *sync.WaitGroup) {
	joinChan := make(chan struct{}, 1)
	go func() {
		// Wait until the goroutines copying stdout/stderr have received EOF
		// and finished, otherwise we might end up sending the exitcode while
		// there is still outstanding output.
		wg.Wait()
		procErr := cmd.Wait()
		exitCodeStr := "0"
		if exiterr, ok := procErr.(*exec.ExitError); ok {
			if waitstatus, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				exitCode := waitstatus.ExitStatus()
				exitCodeStr = strconv.Itoa(exitCode)
				glog.Infof("Exec process exit code: %d", exitCode)
			}
		} else if procErr != nil {
			glog.Warningf("Exec error waiting for process: %v", procErr)
		}
		_ = ws.WriteMsg(wsstream.ExitCodeChan, []byte(exitCodeStr))
		joinChan <- struct{}{}
	}()

	select {
	case <-ws.Closed():
		if cmd.Process != nil {
			cmd.Process.Kill()
			glog.Infoln("Exec WS stream closed, killed process")
		}
	case <-joinChan:
		glog.Info("Exec process ended")
	}
}
