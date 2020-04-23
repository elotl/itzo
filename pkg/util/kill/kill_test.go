package kill

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type MockProcessHandler struct {
	Processes     []Process
	LiveProcesses []Process
	KillList      []int
	ListFailure   bool
	KillFailure   bool
}

func (p *MockProcessHandler) ListProcesses() ([]Process, error) {
	if p.ListFailure {
		return nil, fmt.Errorf("mock ListProcesses() failure")
	}
	ret := make([]Process, len(p.Processes))
	copy(ret, p.Processes)
	return ret, nil
}

func (p *MockProcessHandler) KillProcess(pid int) error {
	if p.KillFailure {
		return fmt.Errorf("mock KillProcess() failure")
	}
	for _, id := range p.KillList {
		if id == pid {
			return fmt.Errorf("%d has already been killed", pid)
		}
	}
	for _, proc := range p.Processes {
		if proc.Pid == pid {
			p.KillList = append(p.KillList, pid)
			return nil
		}
	}
	return fmt.Errorf("can't find pid %d", pid)
}

//func KillProcessTree(ppid int) error
func TestKillProcessTree(t *testing.T) {
	testCases := []struct {
		parentProc  int
		processes   []Process
		killList    []int
		listFailure bool
		killFailure bool
		failure     bool
	}{
		{
			// One process only.
			parentProc: 10,
			processes: []Process{
				{10, 1, "process"},
			},
			killList:    []int{10},
			listFailure: false,
			killFailure: false,
			failure:     false,
		},
		{
			// One process with two children and one unrelated process.
			parentProc: 10,
			processes: []Process{
				{10, 1, "parent"},
				{20, 10, "child1"},
				{30, 10, "child2"},
				{40, 1, "otherprocess"},
			},
			killList:    []int{10, 20, 30},
			listFailure: false,
			killFailure: false,
			failure:     false,
		},
		{
			// One process with three unrelated processes.
			parentProc: 10,
			processes: []Process{
				{10, 1, "parent"},
				{20, 15, "otherprocess"},
				{30, 15, "otherprocess"},
				{40, 1, "otherprocess"},
			},
			killList:    []int{10},
			listFailure: false,
			killFailure: false,
			failure:     false,
		},
		{
			// No such process.
			parentProc: 5,
			processes: []Process{
				{10, 1, "parent"},
				{20, 15, "child1"},
				{30, 15, "child2"},
				{40, 1, "otherprocess"},
			},
			killList:    []int{},
			listFailure: false,
			killFailure: false,
			failure:     true,
		},
		{
			// 4 levels of processes and two unrelated processes.
			parentProc: 10,
			processes: []Process{
				{5, 1, "otherprocess"},
				{10, 1, "parent"},
				{20, 10, "child"},
				{25, 5, "anotherprocess"},
				{30, 20, "grandchild"},
				{40, 30, "greatgrandchild"},
			},
			killList:    []int{10, 20, 30, 40},
			listFailure: false,
			killFailure: false,
			failure:     false,
		},
		{
			// ListProcesses() failure.
			parentProc:  10,
			processes:   []Process{},
			killList:    []int{},
			listFailure: true,
			killFailure: false,
			failure:     true,
		},
		{
			// KillProcess() failure.
			parentProc: 10,
			processes: []Process{
				{10, 1, "parent"},
				{20, 10, "child1"},
				{30, 10, "child2"},
				{40, 1, "otherprocess"},
			},
			killList:    []int{},
			listFailure: false,
			killFailure: true,
			failure:     true,
		},
	}
	for i, tc := range testCases {
		mph := MockProcessHandler{
			Processes:   append([]Process{}, tc.processes...),
			ListFailure: tc.listFailure,
			KillFailure: tc.killFailure,
		}
		ptk := NewProcessTreeKiller(&mph)
		err := ptk.KillProcessTree(tc.parentProc)
		msg := fmt.Sprintf("test case #%d %+v failed; state: %+v", i, tc, mph)
		if !tc.failure {
			assert.NoError(t, err, msg)
		} else {
			assert.Error(t, err, msg)
		}
		assert.ElementsMatch(t, tc.killList, mph.KillList, msg)
	}
}
