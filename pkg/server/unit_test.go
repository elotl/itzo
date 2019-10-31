package server

import (
	"bytes"
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
	"github.com/elotl/itzo/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/syndtr/gocapability/capability"
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

func TestSetState(t *testing.T) {
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

func TestSetStatus(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	defer os.RemoveAll(tmpdir)
	assert.NoError(t, err)
	u, err := OpenUnit(tmpdir, "foobar")
	assert.NoError(t, err)
	defer u.Destroy()
	state := api.UnitState{
		Terminated: &api.UnitStateTerminated{
			ExitCode:   11,
			FinishedAt: api.Now().Rfc3339Copy(),
		},
	}
	us := api.UnitStatus{
		Name:         "foobar",
		State:        state,
		RestartCount: 123,
		Image:        "foobar-img:latest",
	}
	err = u.SetStatus(&us)
	assert.NoError(t, err)
	status, err := u.GetStatus()
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, us, *status)
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
			nil, 0, 0, nil, inr, &stdout, nil, api.RestartPolicyNever)
		ch <- err
	}()
	start := time.Now()
	for {
		status, err := unit.GetStatus()
		assert.NoError(t, err)
		assert.True(t,
			time.Now().Before(start.Add(5*time.Second)),
			"Timed out waiting for unit to start running")
		if status.State.Running != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
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
			nil, 0, 0, nil, nil, nil, nil, api.RestartPolicyAlways)
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
			nil, 0, 0, nil, nil, nil, nil, api.RestartPolicyNever)
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
			nil, 0, 0, nil, nil, nil, nil, api.RestartPolicyOnFailure)
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
			nil, 0, 0, nil, nil, nil, nil, api.RestartPolicyOnFailure)
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

func TestMaybeBackoff(t *testing.T) {
	sleep = func(d time.Duration) {}
	// No error.
	backoff := 1 * time.Second
	runningTime := BACKOFF_RESET_TIME / 2
	maybeBackOff(nil, []string{"mycmd"}, &backoff, runningTime)
	assert.Equal(t, backoff, 1*time.Second)
	// Error.
	err := fmt.Errorf("Testing maybeBackOff()")
	backoff = 1 * time.Second
	maybeBackOff(err, []string{"mycmd"}, &backoff, runningTime)
	assert.Equal(t, backoff, 2*time.Second)
	// No error, backoff needs to be reset.
	backoff = 1 * time.Second
	runningTime = BACKOFF_RESET_TIME * 2
	maybeBackOff(nil, []string{"mycmd"}, &backoff, runningTime)
	assert.Equal(t, backoff, 1*time.Second)
	// Error, backoff needs to be reset.
	backoff = 1 * time.Second
	runningTime = BACKOFF_RESET_TIME * 2
	maybeBackOff(err, []string{"mycmd"}, &backoff, runningTime)
	assert.Equal(t, backoff, 1*time.Second)
}

func int64ptr(i int64) *int64 {
	return &i
}

//getUser(lookup util.UserLookup) (uint32, uint32, []uint32, string, error)
func TestGetUser(t *testing.T) {
	unit := Unit{}
	unit.config = &Config{
		User: "foobar",
	}
	ful := util.FakeUserLookup{}
	ful.Uid = 1234
	ful.UidGid = 5678
	// Looking up user from image config.
	uid, gid, groups, _, err := unit.GetUser(&ful)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1234), uid)
	assert.Equal(t, uint32(5678), gid)
	assert.Empty(t, groups)
	// Looking up user from pod security context.
	unit.securityContext = &securityContext{
		PodSecurityContext: api.PodSecurityContext{
			RunAsUser:          int64ptr(1111),
			RunAsGroup:         int64ptr(2222),
			SupplementalGroups: []int64{1, 2, 3},
		},
	}
	uid, gid, groups, _, err = unit.GetUser(&ful)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1111), uid)
	assert.Equal(t, uint32(2222), gid)
	assert.ElementsMatch(t, []uint32{1, 2, 3}, groups)
	// Looking up user from unit security context.
	unit.securityContext.SecurityContext = api.SecurityContext{
		RunAsUser:  int64ptr(3333),
		RunAsGroup: int64ptr(4444),
	}
	uid, gid, groups, _, err = unit.GetUser(&ful)
	assert.NoError(t, err)
	assert.Equal(t, uint32(3333), uid)
	assert.Equal(t, uint32(4444), gid)
	assert.ElementsMatch(t, []uint32{1, 2, 3}, groups)
	// Error looking up user.
	ful.UidErr = fmt.Errorf("Testing user lookup error")
	_, _, _, _, err = unit.GetUser(&ful)
	assert.Error(t, err)
}

//getCapabilities() ([]string, error)
func TestGetCapabilities(t *testing.T) {
	unit := Unit{}
	// Default capabilities.
	caps, err := unit.getCapabilities()
	assert.NoError(t, err)
	assert.ElementsMatch(t, defaultCapabilities, caps)
	// Add capability.
	unit.securityContext = &securityContext{
		SecurityContext: api.SecurityContext{
			Capabilities: &api.Capabilities{
				Add:  []string{"CAP_NET_ADMIN"},
				Drop: []string{},
			},
		},
	}
	caps, err = unit.getCapabilities()
	assert.NoError(t, err)
	assert.ElementsMatch(t, append(defaultCapabilities, "CAP_NET_ADMIN"), caps)
	// Drop capability.
	unit.securityContext = &securityContext{
		SecurityContext: api.SecurityContext{
			Capabilities: &api.Capabilities{
				Add:  []string{},
				Drop: []string{"CAP_CHOWN"},
			},
		},
	}
	caps, err = unit.getCapabilities()
	assert.NoError(t, err)
	assert.NotContains(t, caps, "CAP_CHOWN")
	for _, c := range defaultCapabilities {
		if c == "CAP_CHOWN" {
			continue
		}
		assert.Contains(t, caps, c)
	}
	// Add and drop capabilities.
	unit.securityContext = &securityContext{
		SecurityContext: api.SecurityContext{
			Capabilities: &api.Capabilities{
				Add:  []string{"CAP_NET_ADMIN", "CAP_MKNOD"},
				Drop: []string{"CAP_NET_BIND_SERVICE", "CAP_NET_RAW"},
			},
		},
	}
	caps, err = unit.getCapabilities()
	assert.NoError(t, err)
	assert.NotContains(t, caps, "CAP_NET_BIND_SERVICE")
	assert.NotContains(t, caps, "CAP_NET_RAW")
	assert.Contains(t, caps, "CAP_NET_ADMIN")
	for _, c := range defaultCapabilities {
		if c == "CAP_NET_BIND_SERVICE" || c == "CAP_NET_RAW" {
			continue
		}
		assert.Contains(t, caps, c)
	}
	// Drop all.
	unit.securityContext = &securityContext{
		SecurityContext: api.SecurityContext{
			Capabilities: &api.Capabilities{
				Add:  []string{},
				Drop: []string{"ALL"},
			},
		},
	}
	caps, err = unit.getCapabilities()
	assert.NoError(t, err)
	assert.Empty(t, caps)
	// Add all.
	unit.securityContext = &securityContext{
		SecurityContext: api.SecurityContext{
			Capabilities: &api.Capabilities{
				Add:  []string{"ALL"},
				Drop: []string{},
			},
		},
	}
	caps, err = unit.getCapabilities()
	assert.NoError(t, err)
	assert.Len(t, caps, 38)
	for _, c := range defaultCapabilities {
		assert.Contains(t, caps, c)
	}
}

//mapCapabilities(keys []string) []capability.Cap
//mapUintptrCapabilities(keys []string) []uintptr
func TestMapCapabilities(t *testing.T) {
	type testCase struct {
		stringCaps []string
		caps       []capability.Cap
	}
	testCases := []testCase{
		{
			[]string{"CAP_NET_ADMIN", "CAP_MKNOD", "CAP_NET_BIND_SERVICE", "CAP_NET_RAW"},
			[]capability.Cap{capability.CAP_NET_ADMIN, capability.CAP_MKNOD, capability.CAP_NET_BIND_SERVICE, capability.CAP_NET_RAW},
		},
		{
			[]string{"CAP_FOOBAR", "CAP_MKNOD", "CAP_NET_BIND_SERVICE", "CAP_NET_RAW"},
			[]capability.Cap{capability.CAP_MKNOD, capability.CAP_NET_BIND_SERVICE, capability.CAP_NET_RAW},
		},
		{
			[]string{},
			[]capability.Cap{},
		},
	}
	for _, tc := range testCases {
		caps := mapCapabilities(tc.stringCaps)
		assert.Len(t, caps, len(tc.caps))
		assert.ElementsMatch(t, tc.caps, caps)
		uintptrSet := make([]uintptr, len(tc.caps))
		for i, c := range tc.caps {
			uintptrSet[i] = uintptr(c)
		}
		uintptrCaps := mapUintptrCapabilities(tc.stringCaps)
		assert.Len(t, uintptrCaps, len(uintptrSet))
		assert.ElementsMatch(t, uintptrSet, uintptrCaps)
	}
}

func TestMakeHostname(t *testing.T) {
	cases := [][]string{
		{"foo", "foo"},
		{"default_foo", "foo"},
		{"default_foo_bar", "foo_bar"},
		{"reallyreallyreallyreallyreallyreallylongnamespace_andafairlylongname", "andafairlylongname"},
		{"foobarreallyreallyreallyreallyreallyreallyreallyreallyreallyreallyreallylongname", "foobarreallyreallyreallyreallyreallyreallyreallyreallyreallyrea"},
	}
	for i, tc := range cases {
		result := makeHostname(tc[0])
		assert.Equal(t, tc[1], result, "failed test case %d: %s -> %s", i, tc[0], result)
	}
}
