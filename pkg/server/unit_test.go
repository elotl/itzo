package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestOpenUnit(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	defer os.RemoveAll(tmpdir)
	assert.Nil(t, err)
	u, err := OpenUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer u.Close()
	uu, err := OpenUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer uu.Close()
	assert.Equal(t, u.Name, uu.Name)
	assert.Equal(t, u.Directory, uu.Directory)
}

func TestGetRootfs(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	defer os.RemoveAll(tmpdir)
	assert.Nil(t, err)
	u, err := OpenUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer u.Close()
	isEmpty, err := isEmptyDir(u.GetRootfs())
	assert.Nil(t, err)
	assert.True(t, isEmpty)
}

func TestStatus(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	defer os.RemoveAll(tmpdir)
	assert.NoError(t, err)
	u, err := OpenUnit(tmpdir, "foobar")
	assert.NoError(t, err)
	defer u.Close()
	waiting := api.UnitState{
		Waiting: &api.UnitStateWaiting{
			Reason:        "testing waiting unit state",
			LaunchFailure: false,
		},
	}
	running := api.UnitState{
		Running: &api.UnitStateRunning{
			StartedAt: api.Now(),
		},
	}
	terminated := api.UnitState{
		Terminated: &api.UnitStateTerminated{
			ExitCode:   0,
			FinishedAt: api.Now(),
		},
	}
	for _, s := range []api.UnitState{waiting, running, terminated} {
		err = u.SetState(s)
		assert.NoError(t, err)
		status, err := u.GetStatus()
		assert.NoError(t, err)
		if s.Waiting != nil {
			assert.NotNil(t, status.State.Waiting)
			assert.Equal(t, s.Waiting.Reason, status.State.Waiting.Reason)
			assert.Equal(t,
				s.Waiting.LaunchFailure, status.State.Waiting.LaunchFailure)
		}
		if s.Running != nil {
			assert.NotNil(t, status.State.Running)
			assert.NotZero(t, status.State.Running.StartedAt)
		}
		if s.Terminated != nil {
			assert.NotNil(t, status.State.Terminated)
			assert.Equal(t,
				s.Terminated.ExitCode, status.State.Terminated.ExitCode)
			assert.NotZero(t, status.State.Terminated.FinishedAt)
		}
	}
}

func TestUnitRestartPolicyAlways(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "itzo-test")
	assert.Nil(t, err)
	defer tmpfile.Close()
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	defer os.RemoveAll(tmpdir)
	unit, err := OpenUnit(tmpdir, "myunit")
	assert.Nil(t, err)
	defer unit.Close()
	ch := make(chan error)
	go func() {
		err = unit.runUnitLoop(
			[]string{"sh", "-c", fmt.Sprintf("echo $$ > %s; exit 1", tmpfile.Name())},
			[]string{}, nil, nil, api.RestartPolicyAlways)
		ch <- err
	}()
	pid := 0
	tries := 0
	select {
	case err = <-ch:
		// Error, runUnitLoop() should not return.
		assert.True(t, false)
	case <-time.After(50 * time.Millisecond):
		tries++
		// Pid has not changed?
		assert.True(t, tries < 20)
		// Wait for pid to change (thus we know the application has been
		// restarted).
		contents, err := ioutil.ReadFile(tmpfile.Name())
		assert.Nil(t, err)
		newPid, err := strconv.Atoi(strings.Trim(string(contents), "\r\n"))
		if err == nil {
			if pid != 0 && newPid != pid {
				break
			}
			pid = newPid
		}
	}
}

func err2rc(t *testing.T, err error) int {
	exiterr, ok := err.(*exec.ExitError)
	assert.True(t, ok)
	ws, ok := exiterr.Sys().(syscall.WaitStatus)
	assert.True(t, ok)
	return ws.ExitStatus()
}

func TestUnitRestartPolicyNever(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "itzo-test")
	assert.Nil(t, err)
	defer tmpfile.Close()
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	defer os.RemoveAll(tmpdir)
	unit, err := OpenUnit(tmpdir, "myunit")
	assert.Nil(t, err)
	defer unit.Close()
	ch := make(chan error)
	go func() {
		err = unit.runUnitLoop(
			[]string{"sh", "-c", fmt.Sprintf("echo $$ > %s; exit 1", tmpfile.Name())},
			[]string{}, nil, nil, api.RestartPolicyNever)
		ch <- err
	}()
	select {
	case err = <-ch:
		assert.NotNil(t, err)
	case <-time.After(10 * time.Second):
		assert.True(t, false)
	}
	// Check return value.
	assert.Equal(t, 1, err2rc(t, err))
	contents, err := ioutil.ReadFile(tmpfile.Name())
	assert.Nil(t, err)
	pid, err := strconv.Atoi(strings.Trim(string(contents), "\r\n"))
	assert.Nil(t, err)
	assert.True(t, pid > 0)
}

func TestUnitRestartPolicyOnFailureHappy(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "itzo-test")
	assert.Nil(t, err)
	defer tmpfile.Close()
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	defer os.RemoveAll(tmpdir)
	unit, err := OpenUnit(tmpdir, "myunit")
	assert.Nil(t, err)
	defer unit.Close()
	ch := make(chan error)
	go func() {
		err = unit.runUnitLoop(
			[]string{"sh", "-c", fmt.Sprintf("echo $$ > %s; exit 0", tmpfile.Name())},
			[]string{}, nil, nil, api.RestartPolicyOnFailure)
		ch <- err
	}()
	select {
	case err = <-ch:
		assert.Nil(t, err)
	case <-time.After(10 * time.Second):
		assert.True(t, false)
	}
	contents, err := ioutil.ReadFile(tmpfile.Name())
	assert.Nil(t, err)
	pid, err := strconv.Atoi(strings.Trim(string(contents), "\r\n"))
	assert.Nil(t, err)
	assert.True(t, pid > 0)
}

func TestUnitRestartPolicyOnFailureSad(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "itzo-test")
	assert.Nil(t, err)
	defer tmpfile.Close()
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	defer os.RemoveAll(tmpdir)
	unit, err := OpenUnit(tmpdir, "myunit")
	assert.Nil(t, err)
	defer unit.Close()
	ch := make(chan error)
	go func() {
		err = unit.runUnitLoop(
			[]string{"sh", "-c", fmt.Sprintf("echo $$ > %s; exit 1", tmpfile.Name())},
			[]string{}, nil, nil, api.RestartPolicyOnFailure)
		ch <- err
	}()
	pid := 0
	tries := 0
	select {
	case err = <-ch:
		// Error, runUnitLoop() should not return.
		assert.True(t, false)
	case <-time.After(50 * time.Millisecond):
		tries++
		// Pid has not changed?
		assert.True(t, tries < 20)
		// Wait for pid to change (thus we know the application has been
		// restarted).
		contents, err := ioutil.ReadFile(tmpfile.Name())
		assert.Nil(t, err)
		newPid, err := strconv.Atoi(strings.Trim(string(contents), "\r\n"))
		if err == nil {
			if pid != 0 && newPid != pid {
				break
			}
			pid = newPid
		}
	}
}

func TestIsUnitExist(t *testing.T) {
	name := randStr(t, 32)
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	defer os.RemoveAll(tmpdir)
	assert.False(t, IsUnitExist(tmpdir, name))
	unit, err := OpenUnit(tmpdir, name)
	assert.Nil(t, err)
	defer unit.Close()
	assert.True(t, IsUnitExist(tmpdir, name))
}

func TestIsUnitExistEmpty(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	defer os.RemoveAll(tmpdir)
	assert.False(t, IsUnitExist(tmpdir, ""))
}
