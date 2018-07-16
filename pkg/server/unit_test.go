package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
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
	defer u.Destroy()
	uu, err := OpenUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer uu.Destroy()
	assert.Equal(t, u.Name, uu.Name)
	assert.Equal(t, u.Directory, uu.Directory)
}

func TestGetRootfs(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	defer os.RemoveAll(tmpdir)
	assert.Nil(t, err)
	u, err := OpenUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer u.Destroy()
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
	defer u.Destroy()
	waiting := api.UnitState{
		Waiting: &api.UnitStateWaiting{
			Reason:       "testing waiting unit state",
			StartFailure: false,
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
		err = u.SetState(s, nil)
		assert.NoError(t, err)
		status, err := u.GetStatus()
		assert.NoError(t, err)
		if s.Waiting != nil {
			assert.NotNil(t, status.State.Waiting)
			assert.Equal(t, s.Waiting.Reason, status.State.Waiting.Reason)
			assert.Equal(t,
				s.Waiting.StartFailure, status.State.Waiting.StartFailure)
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

func TestUnitStdin(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpdir)
	unit, err := OpenUnit(tmpdir, "myunit")
	assert.NoError(t, err)
	defer unit.Destroy()
	ch := make(chan error)
	inr, err := unit.openStdinReader()
	assert.NoError(t, err)
	inw, err := unit.OpenStdinWriter()
	assert.NoError(t, err)
	var stdout bytes.Buffer
	go func() {
		err = unit.runUnitLoop(
			[]string{"cat", "-"},
			[]string{}, 0, 0, inr, &stdout, nil, api.RestartPolicyNever)
		ch <- err
	}()
	msg := []byte("Hello Milpa\n")
	_, err = inw.Write(msg)
	assert.NoError(t, err)
	err = inw.Close()
	assert.NoError(t, err)
	unit.closeStdin()
	select {
	case err = <-ch:
		assert.NoError(t, err)
		assert.Equal(t, msg, stdout.Bytes())
	case <-time.After(1 * time.Second):
		assert.True(t, false, "Timed out waiting for process")
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
	defer unit.Destroy()
	ch := make(chan error)
	go func() {
		err = unit.runUnitLoop(
			[]string{"sh", "-c", fmt.Sprintf("echo $$ > %s; exit 1", tmpfile.Name())},
			[]string{}, 0, 0, nil, nil, nil, api.RestartPolicyAlways)
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
	defer unit.Destroy()
	ch := make(chan error)
	go func() {
		err = unit.runUnitLoop(
			[]string{"sh", "-c", fmt.Sprintf("echo $$ > %s; exit 1", tmpfile.Name())},
			[]string{}, 0, 0, nil, nil, nil, api.RestartPolicyNever)
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
	defer unit.Destroy()
	ch := make(chan error)
	go func() {
		err = unit.runUnitLoop(
			[]string{"sh", "-c", fmt.Sprintf("echo $$ > %s; exit 0", tmpfile.Name())},
			[]string{}, 0, 0, nil, nil, nil, api.RestartPolicyOnFailure)
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
	defer unit.Destroy()
	ch := make(chan error)
	go func() {
		err = unit.runUnitLoop(
			[]string{"sh", "-c", fmt.Sprintf("echo $$ > %s; exit 1", tmpfile.Name())},
			[]string{}, 0, 0, nil, nil, nil, api.RestartPolicyOnFailure)
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
	defer unit.Destroy()
	assert.True(t, IsUnitExist(tmpdir, name))
}

func TestIsUnitExistEmpty(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	defer os.RemoveAll(tmpdir)
	assert.False(t, IsUnitExist(tmpdir, ""))
}

type TestUserLookup struct {
	uid    uint32
	uidGid uint32
	uidErr error
	gid    uint32
	gidErr error
}

func (tul *TestUserLookup) Lookup(username string) (*user.User, error) {
	usr := user.User{}
	usr.Uid = fmt.Sprintf("%d", tul.uid)
	usr.Gid = fmt.Sprintf("%d", tul.uidGid)
	return &usr, tul.uidErr
}

func (tul *TestUserLookup) LookupId(username string) (*user.User, error) {
	usr := user.User{}
	usr.Uid = fmt.Sprintf("%d", tul.uid)
	usr.Gid = fmt.Sprintf("%d", tul.uidGid)
	return &usr, tul.uidErr
}

func (tul *TestUserLookup) LookupGroup(name string) (*user.Group, error) {
	grp := user.Group{}
	grp.Gid = fmt.Sprintf("%d", tul.gid)
	return &grp, tul.uidErr
}

func (tul *TestUserLookup) LookupGroupId(name string) (*user.Group, error) {
	grp := user.Group{}
	grp.Gid = fmt.Sprintf("%d", tul.gid)
	return &grp, tul.uidErr
}

//func lookupUser(userspec string, lookup UserLookup) (uint32, uint32, error)
func TestLookupUser(t *testing.T) {
	type testcase struct {
		user    string
		lookup  UserLookup
		uid     uint32
		gid     uint32
		err     error
		failure bool
	}
	tcs := []testcase{
		{
			user: "",
			lookup: &TestUserLookup{
				uidErr: fmt.Errorf("Testing lookup error"),
				gidErr: fmt.Errorf("Testing lookup error"),
			},
			failure: true,
		},
		{
			user: "myuser",
			lookup: &TestUserLookup{
				uid:    1,
				uidGid: 1,
			},
			uid:     1,
			gid:     1,
			failure: false,
		},
		{
			user: "myuser:mygroup",
			lookup: &TestUserLookup{
				uid: 1,
				gid: 1,
			},
			uid:     1,
			gid:     1,
			failure: false,
		},
		{
			user: "1001",
			lookup: &TestUserLookup{
				uid:    1001,
				uidGid: 1001,
			},
			uid:     1001,
			gid:     1001,
			failure: false,
		},
	}
	for _, tc := range tcs {
		uid, gid, err := lookupUser(tc.user, tc.lookup)
		if tc.failure {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		assert.Equal(t, tc.uid, uid)
		assert.Equal(t, tc.gid, gid)
	}
}
