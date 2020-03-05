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
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"k8s.io/apimachinery/pkg/util/intstr"
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
		assert.True(t, false, "test timed out")
		return
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
	unit.unitConfig = UnitConfig{
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
	unit.unitConfig.SecurityContext = api.SecurityContext{
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
	unit.unitConfig = UnitConfig{
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
	unit.unitConfig = UnitConfig{
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
	unit.unitConfig = UnitConfig{
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
	unit.unitConfig = UnitConfig{
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
	unit.unitConfig = UnitConfig{
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

func mkTestUnit(t *testing.T) (*Unit, func()) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	if err != nil {
		t.FailNow()
	}
	u, err := OpenUnit(tmpdir, "foobar")
	if err != nil {
		os.RemoveAll(tmpdir)
		t.FailNow()
	}
	closer := func() {
		os.RemoveAll(tmpdir)
		u.Destroy()
	}
	return u, closer
}

func TestWatchCmdLivenessReadiness(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		cmd          []string
		livenessCmd  []string
		readinessCmd []string
		isReady      bool
		livenessErr  bool
		cmdErr       bool
	}{
		{
			name:         "no probes",
			cmd:          []string{"/bin/bash", "-c", "sleep 2"},
			livenessCmd:  []string{},
			readinessCmd: []string{},
			isReady:      true,
			livenessErr:  false,
			cmdErr:       false,
		},
		{
			name:         "probes succeed",
			cmd:          []string{"/bin/bash", "-c", "sleep 2"},
			livenessCmd:  []string{"ls", "/"},
			readinessCmd: []string{"ls", "/"},
			isReady:      true,
			livenessErr:  false,
			cmdErr:       false,
		},
		{
			name:         "liveness error",
			cmd:          []string{"/bin/bash", "-c", "sleep 4"},
			livenessCmd:  []string{"/bin/bash", "-c", "sleep 2; exit 1"},
			readinessCmd: []string{"ls", "/"},
			isReady:      true,
			livenessErr:  true,
			cmdErr:       false,
		},
		{
			name:         "readiness error",
			cmd:          []string{"/bin/bash", "-c", "sleep 2"},
			readinessCmd: []string{"/bin/false"},
			isReady:      false,
			livenessErr:  false,
			cmdErr:       false,
		},
		{
			name:        "cmd error, no readiness probe",
			cmd:         []string{"/bin/false"},
			livenessCmd: []string{"ls", "/"},
			isReady:     true,
			livenessErr: false,
			cmdErr:      true,
		},
	}
	for i, tc := range tests {
		// These are time consuming, lets run them in parallel
		// see https://gist.github.com/posener/92a55c4cd441fc5e5e85f27bca008721
		msg := fmt.Sprintf("test %d: %s", i, tc.name)
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u, closer := mkTestUnit(t)
			defer closer()

			cmd := exec.Command(tc.cmd[0], tc.cmd[1:]...)
			var rp *api.Probe
			if len(tc.readinessCmd) > 0 {
				rp = &api.Probe{
					Handler: api.Handler{
						Exec: &api.ExecAction{tc.readinessCmd},
					},
					InitialDelaySeconds: 0,
					PeriodSeconds:       1,
					SuccessThreshold:    1,
					FailureThreshold:    1,
				}
			}
			var lp *api.Probe
			if len(tc.livenessCmd) > 0 {
				lp = &api.Probe{
					Handler: api.Handler{
						Exec: &api.ExecAction{tc.livenessCmd},
					},
					InitialDelaySeconds: 0,
					PeriodSeconds:       1,
					SuccessThreshold:    1,
					FailureThreshold:    1,
				}
			}

			err := cmd.Start()
			assert.NoError(t, err, msg)

			cmdErr, probeErr := u.watchRunningCmd(cmd, nil, rp, lp)
			assert.Equal(t, tc.livenessErr, probeErr != nil, msg)
			assert.Equal(t, tc.cmdErr, cmdErr != nil, msg)
			s, err := u.GetStatus()
			assert.NoError(t, err)
			if s.Started != nil {
				assert.True(t, *s.Started)
			} else {
				assert.Fail(t, "unit status should not be nil")
			}
			assert.Equal(t, tc.isReady, s.Ready)
		})
	}
}

func TestHTTPLivenessProbe(t *testing.T) {
	t.Parallel()
	u, closer := mkTestUnit(t)
	defer closer()

	recordedPath := "UNSET"
	probePath := "/healthy"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordedPath = r.URL.Path
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()
	serverURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	portStr := serverURL.Port()
	port, err := strconv.ParseInt(portStr, 10, 64)
	assert.NoError(t, err)
	cmd := exec.Command("/bin/bash", "-c", "sleep 2")
	lp := &api.Probe{
		Handler: api.Handler{
			HTTPGet: &api.HTTPGetAction{
				Path:   probePath,
				Port:   intstr.FromInt(int(port)),
				Scheme: api.URISchemeHTTP,
			},
		},
		InitialDelaySeconds: 0,
		PeriodSeconds:       1,
		SuccessThreshold:    1,
		FailureThreshold:    1,
	}
	err = cmd.Start()
	assert.NoError(t, err)

	cmdErr, probeErr := u.watchRunningCmd(cmd, nil, nil, lp)
	assert.Equal(t, probePath, recordedPath)
	assert.Nil(t, cmdErr)
	assert.Nil(t, probeErr)
}

func TestStartupProbe(t *testing.T) {
	t.Parallel()
	// create a cmd
	// have a startup prove that fails
	// have a startup probe that succeeds
	// have a command that fails before startup probe returns
	tests := []struct {
		name       string
		cmd        []string
		startupCmd []string
		isStarted  bool
		startupErr bool
		cmdErr     bool
	}{
		{
			name:       "startup probe succeeds",
			cmd:        []string{"/bin/bash", "-c", "sleep 2"},
			startupCmd: []string{"/bin/ls", "/"},
			isStarted:  true,
			startupErr: false,
			cmdErr:     false,
		},
		{
			name:       "startup probe fails",
			cmd:        []string{"/bin/bash", "-c", "sleep 2"},
			startupCmd: []string{"/bin/false"},
			isStarted:  false,
			startupErr: true,
			cmdErr:     false,
		},
		{
			name:       "cmd returns before startup",
			cmd:        []string{"/bin/false"},
			startupCmd: []string{"/bin/bash", "-c", "sleep 2"},
			isStarted:  false,
			startupErr: false,
			cmdErr:     true,
		},
	}
	for i, tc := range tests {
		msg := fmt.Sprintf("test %d: %s", i, tc.name)
		u, closer := mkTestUnit(t)
		defer closer()
		cmd := exec.Command(tc.cmd[0], tc.cmd[1:]...)
		var sp *api.Probe
		if len(tc.startupCmd) > 0 {
			sp = &api.Probe{
				Handler: api.Handler{
					Exec: &api.ExecAction{tc.startupCmd},
				},
				InitialDelaySeconds: 0,
				PeriodSeconds:       1,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			}
		}
		err := cmd.Start()
		assert.NoError(t, err, msg)
		cmdErr, probeErr := u.watchRunningCmd(cmd, sp, nil, nil)
		assert.Equal(t, tc.startupErr, probeErr != nil, msg)
		assert.Equal(t, tc.cmdErr, cmdErr != nil, msg)
		s, err := u.GetStatus()
		assert.NoError(t, err)
		if s.Started == nil {
			// if the unit status says we have no started status
			// ensure that agreees with the test case
			assert.False(t, tc.isStarted)
		} else {
			assert.Equal(t, tc.isStarted, *s.Started)
		}
	}
}

// Tests that a failed probe kills a running cmd
func TestCmdCleanupFailedProbe(t *testing.T) {
	u, closer := mkTestUnit(t)
	defer closer()

	cmd := exec.Command("/bin/bash", "-c", "sleep 20")
	err := cmd.Start()
	testStart := time.Now()
	assert.NoError(t, err)
	probeErr := fmt.Errorf("probe failed")
	keepGoing := u.handleCmdCleanup(cmd, nil, probeErr, api.RestartPolicyNever, testStart)
	procErr := cmd.Wait()
	assert.Error(t, procErr)
	if time.Since(testStart) > 10*time.Second {
		assert.Fail(t, "did not kill command in time")
	}
	assert.False(t, keepGoing)
}
