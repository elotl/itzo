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
	"errors"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/wsstream"
	"os/exec"
)

const (
	wsTTYControlChan = 4
)

func (s *Server) runExec(ws *wsstream.WSReadWriter, params api.ExecParams) {
	writeWSErrorExitcode(ws, "not supported on arm")
	return
}

func (s *Server) runExecCmd(ws *wsstream.WSReadWriter, cmd *exec.Cmd, interactive bool) error {
	return errors.New("not supported on arm")
}

func (s *Server) runExecTTY(ws *wsstream.WSReadWriter, cmd *exec.Cmd, interactive bool) error {
	return errors.New("not supported on arm")
}

