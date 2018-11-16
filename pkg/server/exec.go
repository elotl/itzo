package server

import (
	"encoding/json"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"syscall"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/wsstream"
	"github.com/golang/glog"
	"github.com/kr/pty"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	wsTTYControlChan = 4
)

func (s *Server) runExec(ws *wsstream.WSReadWriter, params api.ExecParams) {
	if len(params.Command) == 0 {
		glog.Errorf("No command specified for exec")
		writeWSError(ws, "No command specified")
		return
	}

	unitName, err := s.podController.GetUnitName(params.UnitName)
	if err != nil {
		glog.Errorf("Getting unit %s: %v", params.UnitName, err)
		writeWSError(ws, err.Error())
		return
	}

	command := params.Command

	// allow us to skip entering namespace for testing
	if !params.SkipNSEnter {
		pid, exists := s.unitMgr.GetPid(unitName)
		if !exists {
			glog.Errorf("Error getting pid for unit %s", unitName)
			writeWSError(ws, "Could not find running process for unit named %s\n", unitName)
			return
		}
		nsenterCmd := []string{
			"/usr/bin/nsenter",
			"-t",
			strconv.Itoa(pid),
			"-p",
			"-u",
			"-m",
		}
		command = append(nsenterCmd, command...)
	}

	cmd := exec.Command(command[0], command[1:]...)
	if params.TTY {
		err = s.runExecTTY(ws, cmd, params.Interactive)
	} else {
		err = s.runExecCmd(ws, cmd, params.Interactive)
	}
	if err != nil {
		glog.Errorf("Error running exec command %v: %v", command, err)
		writeWSError(ws, err.Error())
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
		glog.Errorf("Error starting pty for exec command %+v: %v", cmd, err)
		return err
	}
	defer tty.Close()
	if interactive {
		oldState, err := terminal.MakeRaw(int(tty.Fd()))
		if err != nil {
			glog.Errorf("Error setting up terminal for exec: %v", err)
			return (err)
		}
		defer terminal.Restore(int(tty.Fd()), oldState)

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
				glog.Warning("error unmarshalling pty resize: %s", err)
				// should we send these errors back on stderr?
				return
			}
			if err := pty.Setsize(tty, &s); err != nil {
				glog.Warning("error resizing pty: %s", err)
				return
			}
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
			}
		}
		_ = ws.WriteMsg(wsstream.ExitCodeChan, []byte(exitCodeStr))
		joinChan <- struct{}{}
	}()

	select {
	case <-ws.Closed():
		if cmd.Process != nil {
			cmd.Process.Kill()
			glog.Infoln("killed process")
		}
	case <-joinChan:
		glog.Info("Exec process ended")
	}
}
