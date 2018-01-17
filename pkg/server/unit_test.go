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
	assert.Nil(t, err)
	u, err := OpenUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer u.Close()
	for _, s := range []UnitStatus{UnitStatusCreated, UnitStatusRunning, UnitStatusFailed, UnitStatusSucceeded} {
		err = u.SetStatus(s)
		assert.Nil(t, err)
		ss, err := u.GetStatus()
		assert.Nil(t, err)
		assert.Equal(t, s, ss)
	}
}

func TestStringToRestartPolicy(t *testing.T) {
	policyStrs := []string{"always", "never", "onfailure"}
	for _, pstr := range policyStrs {
		assert.Equal(t, pstr, RestartPolicyToString(StringToRestartPolicy(pstr)))
	}
	assert.Equal(t, StringToRestartPolicy("foobar"), RESTART_POLICY_ALWAYS)
}

func TestRestartPolicyToString(t *testing.T) {
	policies := []RestartPolicy{
		RESTART_POLICY_ALWAYS,
		RESTART_POLICY_NEVER,
		RESTART_POLICY_ONFAILURE,
	}
	for _, p := range policies {
		assert.Equal(t, p, StringToRestartPolicy(RestartPolicyToString(p)))
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
			[]string{}, nil, nil, RESTART_POLICY_ALWAYS)
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
			[]string{}, nil, nil, RESTART_POLICY_NEVER)
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
			[]string{}, nil, nil, RESTART_POLICY_ONFAILURE)
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
			[]string{}, nil, nil, RESTART_POLICY_ONFAILURE)
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
