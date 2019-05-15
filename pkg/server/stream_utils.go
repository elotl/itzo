package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elotl/wsstream"
	"github.com/golang/glog"
)

func (s *Server) doUpgrade(w http.ResponseWriter, r *http.Request) (*wsstream.WSReadWriter, error) {
	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		serverError(w, err)
		return nil, err
	}
	ws := &wsstream.WSReadWriter{
		WSStream: wsstream.NewWSStream(conn),
	}
	return ws, nil
}

func writeWSError(ws *wsstream.WSReadWriter, format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	err := ws.WriteMsg(wsstream.StderrChan, []byte(msg))
	if err != nil {
		glog.Errorln("Error writing error to websocket, reporting it here:", msg)
	}
}

func writeWSErrorExitcode(ws *wsstream.WSReadWriter, format string, a ...interface{}) {
	writeWSError(ws, format, a...)
	err := ws.WriteMsg(wsstream.ExitCodeChan, []byte("-1"))
	if err != nil {
		glog.Errorf("Error writing exitcode to websocket: %v", err)
	}
}

func getInitialParams(ws *wsstream.WSReadWriter, params interface{}) error {
	select {
	case <-ws.Closed():
		return fmt.Errorf("connection closed before first parameter")
	case paramsJson := <-ws.ReadMsg():
		err := json.Unmarshal(paramsJson, params)
		if err != nil {
			msg := fmt.Sprintf("Error reading parameters %v", err)
			glog.Error(msg)
			ws.WriteMsg(wsstream.StderrChan, []byte(msg))
			return err
		}
	}
	return nil
}
