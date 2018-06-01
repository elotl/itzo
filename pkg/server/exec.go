package server

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/wsstream"
	"github.com/golang/glog"
	quote "github.com/kballard/go-shellquote"
	"github.com/kr/pty"
	"golang.org/x/crypto/ssh/terminal"
)

func (s *Server) runExec(ws *wsstream.WSReadWriter, params api.ExecParams) {
	cmdArray, err := quote.Split(params.Command)
	if err != nil {
		msg := fmt.Sprintf("error parsing exec command: %v", err)
		_ = ws.WriteMsg(wsstream.StderrChan, []byte(msg))
	}

	cmd := exec.Command(cmdArray[0], cmdArray[1:]...)
	if params.TTY {
		err = s.runExecTTY(ws, cmd, params.Interactive)
	} else {
		err = s.runExecCmd(ws, cmd, params.Interactive)
	}
	if err != nil {
		err := ws.WriteMsg(2, []byte(err.Error()))
		if err != nil {
			glog.Errorln("Error writing error to websocket, reporting it here", err)
		}
	}
}

func (s *Server) runExecCmd(ws *wsstream.WSReadWriter, cmd *exec.Cmd, interactive bool) error {
	if interactive {
		wsStdinReader := ws.CreateReader(0)
		inPipe, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		go io.Copy(inPipe, wsStdinReader)
	}

	wsStdoutWriter := ws.CreateWriter(wsstream.StdoutChan)
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go io.Copy(wsStdoutWriter, outPipe)

	wsStderrWriter := ws.CreateWriter(wsstream.StderrChan)
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	go io.Copy(wsStderrWriter, errPipe)

	err = cmd.Start()
	if err != nil {
		return err
	}

	go ws.RunDispatch()

	waitForFinished(ws, cmd)
	return nil
}

func (s *Server) runExecTTY(ws *wsstream.WSReadWriter, cmd *exec.Cmd, interactive bool) error {
	tty, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer tty.Close()
	if interactive {
		oldState, err := terminal.MakeRaw(int(tty.Fd()))
		if err != nil {
			return (err)
		}
		defer terminal.Restore(int(tty.Fd()), oldState)

		wsStdinReader := ws.CreateReader(0)
		go func() {
			io.Copy(tty, wsStdinReader)
		}()

		// handle resize terminal messages
		termChanges := ws.CreateReader(4)
		go func() {
			for {
				buf := make([]byte, 32*1024)
				n, err := termChanges.Read(buf)
				if err != nil {
					return
				}
				var s pty.Winsize
				err = json.Unmarshal(buf[0:n], &s)
				if err != nil {
					glog.Warning("error unmarshalling pty resize: %s", err)
					continue
				}
				if err := pty.Setsize(tty, &s); err != nil {
					glog.Warning("error resizing pty: %s", err)
					continue
				}
			}
		}()
	}

	wsStdoutWriter := ws.CreateWriter(1)
	go func() {
		io.Copy(wsStdoutWriter, tty)
	}()

	go ws.RunDispatch()
	waitForFinished(ws, cmd)
	return nil
}

func waitForFinished(ws *wsstream.WSReadWriter, cmd *exec.Cmd) {
	joinChan := make(chan struct{}, 1)
	go func() {
		cmd.Wait()
		//
		// todo, get the exit code of the process and write it to exit
		// channel
		//

		// if we don't wait here the websocket closes
		// before we can flush the final output
		time.Sleep(1 * time.Second)
		joinChan <- struct{}{}
	}()
	select {
	case <-ws.Closed():
		if cmd.Process != nil {
			cmd.Process.Kill()
			fmt.Println("killed process")
		} else {
			fmt.Println("proc is nil")
		}
	case <-joinChan:
		fmt.Println("process ended")
	}
}
