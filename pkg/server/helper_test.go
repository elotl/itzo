package server

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetSetRestartPolicyDefault(t *testing.T) {
	env := []string{}
	assert.Equal(t, RESTART_POLICY_ALWAYS, GetRestartPolicy(env))
}

func TestGetSetRestartPolicy(t *testing.T) {
	policies := []RestartPolicy{
		RESTART_POLICY_ALWAYS,
		RESTART_POLICY_NEVER,
		RESTART_POLICY_ONFAILURE,
	}
	for _, p := range policies {
		env := []string{}
		SetRestartPolicy(&env, p)
		assert.Equal(t, p, GetRestartPolicy(env))
	}
}

func TestUnitRestartPolicyAlways(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "itzo-test")
	assert.Nil(t, err)
	defer tmpfile.Close()
	ch := make(chan error)
	go func() {
		err = runUnit(
			[]string{"sh", "-c", fmt.Sprintf("echo $$ > %s; exit 1", tmpfile.Name())},
			[]string{}, nil, nil, RESTART_POLICY_ALWAYS)
		ch <- err
	}()
	pid := 0
	tries := 0
	select {
	case err = <-ch:
		// Error, runUnit() should not return.
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
	ch := make(chan error)
	go func() {
		err = runUnit(
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
	ch := make(chan error)
	go func() {
		err = runUnit(
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
	ch := make(chan error)
	go func() {
		err = runUnit(
			[]string{"sh", "-c", fmt.Sprintf("echo $$ > %s; exit 1", tmpfile.Name())},
			[]string{}, nil, nil, RESTART_POLICY_ONFAILURE)
		ch <- err
	}()
	pid := 0
	tries := 0
	select {
	case err = <-ch:
		// Error, runUnit() should not return.
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
