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
