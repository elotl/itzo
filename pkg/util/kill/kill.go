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

package kill

import (
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/mitchellh/go-ps"
)

type Process struct {
	Pid        int
	PPid       int
	Executable string
}

type ProcessHandler interface {
	ListProcesses() ([]Process, error)
	KillProcess(pid int) error
}

type OSProcessHandler struct{}

func (p *OSProcessHandler) ListProcesses() ([]Process, error) {
	ps, err := ps.Processes()
	if err != nil {
		return nil, err
	}
	processes := make([]Process, 0, len(ps))
	for _, p := range ps {
		processes = append(processes, Process{
			Pid:        p.Pid(),
			PPid:       p.PPid(),
			Executable: p.Executable(),
		})
	}
	return processes, nil
}

func (p *OSProcessHandler) KillProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

type ProcessTreeKiller struct {
	handler ProcessHandler
}

func NewProcessTreeKiller(handler ProcessHandler) *ProcessTreeKiller {
	return &ProcessTreeKiller{
		handler: handler,
	}
}

func (ptk *ProcessTreeKiller) KillProcessTree(ppid int) error {
	processes, err := ptk.handler.ListProcesses()
	if err != nil {
		return err
	}
	return ptk.killProcessTree(ppid, processes, nil)
}

func copyExcept(processes []Process, except int) []Process {
	result := make([]Process, 0, len(processes)-1)
	for i, prc := range processes {
		if i != except {
			result = append(result, prc)
		}
	}
	return result
}

func (ptk *ProcessTreeKiller) killProcessTree(ppid int, processes []Process, result error) error {
	for i, proc := range processes {
		if proc.PPid != ppid {
			continue
		}
		reduced := copyExcept(processes, i)
		err := ptk.killProcessTree(proc.Pid, reduced, result)
		if err != nil {
			result = multierror.Append(result, err)
		}
	}
	err := ptk.handler.KillProcess(ppid)
	if err != nil {
		result = multierror.Append(result, err)
	}
	return result
}
