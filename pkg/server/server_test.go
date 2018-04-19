package server

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/stretchr/testify/assert"
)

var (
	// UUgh, not sure why I made this a global var possibly because
	// we used to use gorilla mux??? Either way, it should go away   :/
	s             Server
	runFunctional = flag.Bool("functional", false, "run functional tests")
)

// This will ensure all the helper processes and their children get terminated
// before the main process exits.
func killChildren() {
	// Set of pids.
	var pids map[int]interface{} = make(map[int]interface{})
	pids[os.Getpid()] = nil

	d, err := os.Open("/proc")
	if err != nil {
		return
	}
	defer d.Close()

	for {
		fis, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}

		for _, fi := range fis {
			if !fi.IsDir() {
				continue
			}
			name := fi.Name()
			if name[0] < '0' || name[0] > '9' {
				continue
			}
			pid64, err := strconv.ParseInt(name, 10, 0)
			if err != nil {
				continue
			}
			pid := int(pid64)
			statPath := fmt.Sprintf("/proc/%s/stat", name)
			dataBytes, err := ioutil.ReadFile(statPath)
			if err != nil {
				continue
			}
			data := string(dataBytes)
			binStart := strings.IndexRune(data, '(') + 1
			binEnd := strings.IndexRune(data[binStart:], ')')
			data = data[binStart+binEnd+2:]
			var state int
			var ppid int
			var pgrp int
			var sid int
			_, _ = fmt.Sscanf(data, "%c %d %d %d", &state, &ppid, &pgrp, &sid)
			_, ok := pids[ppid]
			if ok {
				syscall.Kill(pid, syscall.SIGKILL)
				// Kill any children of this process too.
				pids[pid] = nil
			}
		}
	}
}

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags
	var appcmdline = flag.String("exec", "", "Command for starting a unit")
	var rootdir = flag.String("rootdir", DEFAULT_ROOTDIR, "Base dir for units")
	var unit = flag.String("unit", "myunit", "Unit name")
	var rp = flag.String("restartpolicy", string(api.RestartPolicyAlways), "Restart policy")
	flag.Parse()
	if *appcmdline != "" {
		policy := api.RestartPolicy(*rp)
		StartUnit(*rootdir, *unit, strings.Split(*appcmdline, " "), policy)
		os.Exit(0)
	}
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	if err != nil {
		panic("Error creating temporary directory")
	}
	s = Server{
		env:            EnvStore{},
		installRootdir: tmpdir,
	}
	s.getHandlers()
	ret := m.Run()
	// Engineering: where killing children is how you keep things clean.
	killChildren()
	os.Exit(ret)
}

func sendRequest(t *testing.T, method, url string, body io.Reader) *httptest.ResponseRecorder {
	req, err := http.NewRequest(method, url, body)
	assert.Nil(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	return rr
}

func TestPingHandler(t *testing.T) {
	rr := sendRequest(t, "GET", "/rest/v1/ping", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "pong", rr.Body.String())
}

func TestGetFile(t *testing.T) {
	f, err := ioutil.TempFile("", "itzo-test")
	assert.NoError(t, err)
	defer f.Close()
	contents := "123\n456\n789\n0\n"
	_, err = f.Write([]byte(contents))
	assert.NoError(t, err)
	data := url.Values{}
	data.Set("path", f.Name())
	// Test getting the whole thing
	path := "/rest/v1/file/?" + data.Encode()
	rr := sendRequest(t, "GET", path, strings.NewReader(""))
	assert.Equal(t, http.StatusOK, rr.Code)
	responseBody := rr.Body.String()
	assert.Equal(t, contents, responseBody)

	// Test getting a couple of bytes
	data.Set("bytes", "6")
	path = "/rest/v1/file/?" + data.Encode()
	rr = sendRequest(t, "GET", path, strings.NewReader(""))
	assert.Equal(t, http.StatusOK, rr.Code)
	responseBody = rr.Body.String()
	assert.Equal(t, "789\n0\n", responseBody)

	// Test getting a couple of lines
	data.Set("bytes", "0")
	data.Set("lines", "2")
	path = "/rest/v1/file/?" + data.Encode()
	rr = sendRequest(t, "GET", path, strings.NewReader(""))
	assert.Equal(t, http.StatusOK, rr.Code)
	responseBody = rr.Body.String()
	assert.Equal(t, "789\n0\n", responseBody)
}

func TestVersionHandler(t *testing.T) {
	rr := sendRequest(t, "GET", "/rest/v1/version", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.NotNil(t, rr.Body)
	assert.NotEmpty(t, rr.Body.String())
}

func randStr(t *testing.T, n int) string {
	s := ""
	for len(s) < n {
		buf := make([]byte, 16)
		m, err := rand.Read(buf)
		buf = buf[:m]
		assert.Nil(t, err)
		for _, b := range buf {
			if (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') {
				s = s + string(b)
				if len(s) == n {
					break
				}
			}
		}
	}
	return s
}

func createUnit(t *testing.T) *api.PodParameters {
	units := make([]api.Unit, 1)
	units[0] = api.Unit{
		Name:    randStr(t, 16),
		Image:   "library/alpine",
		Command: "echo Hello Milpa",
	}
	params := api.PodParameters{
		Spec: api.PodSpec{
			Units:         units,
			RestartPolicy: api.RestartPolicyAlways,
		},
	}
	buf, err := json.Marshal(&params)
	assert.NoError(t, err)
	body := strings.NewReader(string(buf))
	rr := sendRequest(t, "POST", "/rest/v1/updatepod", body)
	assert.Equal(t, http.StatusOK, rr.Code)
	return &params
}

func updateUnit(t *testing.T, params *api.PodParameters) {
	buf, err := json.Marshal(params)
	assert.NoError(t, err)
	body := strings.NewReader(string(buf))
	rr := sendRequest(t, "POST", "/rest/v1/updatepod", body)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestUpdateHandler(t *testing.T) {
	if !*runFunctional {
		return
	}
	_ = createUnit(t)
}

func TestUpdateHandlerAddVolume(t *testing.T) {
	if !*runFunctional {
		return
	}
	params := createUnit(t)
	volume := api.Volume{
		Name: randStr(t, 8),
		VolumeSource: api.VolumeSource{
			EmptyDir: &api.EmptyDir{},
		},
	}
	params.Spec.Volumes = []api.Volume{volume}
	buf, err := json.Marshal(params)
	assert.NoError(t, err)
	body := strings.NewReader(string(buf))
	rr := sendRequest(t, "POST", "/rest/v1/updatepod", body)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestUpdateHandlerAddUnit(t *testing.T) {
	if !*runFunctional {
		return
	}
	params := createUnit(t)
	unit := api.Unit{
		Name:    randStr(t, 8),
		Image:   "library/alpine",
		Command: "echo Hello World",
	}
	params.Spec.Units = append(params.Spec.Units, unit)
	buf, err := json.Marshal(params)
	assert.NoError(t, err)
	body := strings.NewReader(string(buf))
	rr := sendRequest(t, "POST", "/rest/v1/updatepod", body)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestStatusHandler(t *testing.T) {
	if !*runFunctional {
		return
	}
	_ = createUnit(t)
	path := "/rest/v1/status"
	timeout := time.Now().Add(5 * time.Second)
	var reply api.PodStatusReply
	for time.Now().Before(timeout) {
		rr := sendRequest(t, "GET", path, nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		err := json.Unmarshal(rr.Body.Bytes(), &reply)
		assert.NoError(t, err)
		assert.Len(t, reply.UnitStatuses, 1)
		if reply.UnitStatuses[0].State.Running != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.NotNil(t, reply.UnitStatuses[0].State.Running)
}

func TestStatusHandlerFailed(t *testing.T) {
	if !*runFunctional {
		return
	}
	params := createUnit(t)
	params.Spec.Units = []api.Unit{
		api.Unit{
			Name:    params.Spec.Units[0].Name,
			Command: "ls /does_not_exist",
		},
	}
	params.Spec.RestartPolicy = api.RestartPolicyNever
	updateUnit(t, params)
	var reply api.PodStatusReply
	timeout := time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		rr := sendRequest(t, "GET", "/rest/v1/status", nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		err := json.Unmarshal(rr.Body.Bytes(), &reply)
		assert.NoError(t, err)
		assert.Len(t, reply.UnitStatuses, 1)
		if reply.UnitStatuses[0].State.Terminated != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.NotNil(t, reply.UnitStatuses[0].State.Terminated)
	assert.NotZero(t, reply.UnitStatuses[0].State.Terminated.ExitCode)
	assert.NotZero(t, reply.UnitStatuses[0].State.Terminated.FinishedAt)
}

func TestStatusHandlerLaunchFailure(t *testing.T) {
	if !*runFunctional {
		return
	}
	params := createUnit(t)
	params.Spec.Units = []api.Unit{
		api.Unit{
			Name:    params.Spec.Units[0].Name,
			Command: "/does_not_exist",
		},
	}
	params.Spec.RestartPolicy = api.RestartPolicyNever
	updateUnit(t, params)
	var reply api.PodStatusReply
	timeout := time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		rr := sendRequest(t, "GET", "/rest/v1/status", nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		err := json.Unmarshal(rr.Body.Bytes(), &reply)
		assert.NoError(t, err)
		assert.Len(t, reply.UnitStatuses, 1)
		if reply.UnitStatuses[0].State.Waiting != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.NotNil(t, reply.UnitStatuses[0].State.Waiting)
	assert.True(t, reply.UnitStatuses[0].State.Waiting.LaunchFailure)
}

func TestStatusHandlerSucceeded(t *testing.T) {
	if !*runFunctional {
		return
	}
	params := createUnit(t)
	params.Spec.Units = []api.Unit{
		api.Unit{
			Name:    params.Spec.Units[0].Name,
			Command: "/does_not_exist",
		},
	}
	params.Spec.RestartPolicy = api.RestartPolicyNever
	updateUnit(t, params)
	var reply api.PodStatusReply
	timeout := time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		rr := sendRequest(t, "GET", "/rest/v1/status", nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		err := json.Unmarshal(rr.Body.Bytes(), &reply)
		assert.NoError(t, err)
		assert.Len(t, reply.UnitStatuses, 1)
		if reply.UnitStatuses[0].State.Terminated != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.NotNil(t, reply.UnitStatuses[0].State.Terminated)
	assert.Zero(t, reply.UnitStatuses[0].State.Terminated.ExitCode)
	assert.NotZero(t, reply.UnitStatuses[0].State.Terminated.FinishedAt)
}

func TestGetLogs(t *testing.T) {
	if !*runFunctional {
		return
	}
	params := createUnit(t)
	name := params.Spec.Units[0].Name
	var lines []string
	timeout := time.Now().Add(3 * time.Second)
	for time.Now().Before(timeout) {
		path := fmt.Sprintf("/rest/v1/logs/%s", name)
		rr := sendRequest(t, "GET", path, nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		lines = strings.Split(rr.Body.String(), "\n")
		if len(lines) >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.True(t, 2 <= len(lines))
}

func TestGetLogsLines(t *testing.T) {
	if !*runFunctional {
		return
	}
	params := createUnit(t)
	params.Spec.Units[0].Command = "sh -c yes | head -n10"
	buf, err := json.Marshal(params)
	assert.NoError(t, err)
	body := strings.NewReader(string(buf))
	rr := sendRequest(t, "POST", "/rest/v1/updatepod", body)
	assert.Equal(t, http.StatusOK, rr.Code)
	var lines []string
	timeout := time.Now().Add(3 * time.Second)
	for time.Now().Before(timeout) {
		path := "/rest/v1/logs/yes?lines=3"
		rr := sendRequest(t, "GET", path, nil)
		assert.Equal(t, http.StatusOK, rr.Code)
		lines = strings.Split(rr.Body.String(), "\n")
		if len(lines) >= 4 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.True(t, 4 <= len(lines))
	for _, line := range lines[:len(lines)-1] {
		assert.Equal(t, "y", line)
	}
}
