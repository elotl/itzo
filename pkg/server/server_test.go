package server

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// UUgh, not sure why I made this a global var possibly because
// we used to use gorilla mux??? Either way, it should go away   :/
var s Server

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
	var rp = flag.String("restartpolicy", "always", "Restart policy")
	flag.Parse()
	if *appcmdline != "" {
		policy := StringToRestartPolicy(*rp)
		StartUnit(*rootdir, *unit, strings.Split(*appcmdline, " "), policy)
		os.Exit(0)
	}
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	if err != nil {
		panic("Error creating temporary directory")
	}
	s = Server{
		env:            StringMap{data: map[string]string{}},
		installRootdir: tmpdir,
	}
	s.getHandlers()
	ret := m.Run()
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

func assertFileHasContents(t *testing.T, filepath, expectedContent string) {
	f, err := os.Open(filepath)
	assert.Nil(t, err)
	defer f.Close()
	buf := make([]byte, 10000)
	_, err = f.Read(buf)
	assert.Nil(t, err)
	fileContent := string(buf[:]) // This ends up looking like
	assert.Contains(t, fileContent, expectedContent)
}

func TestPingHandler(t *testing.T) {
	rr := sendRequest(t, "GET", "/rest/v1/ping", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "pong", rr.Body.String())
}

func TestEnvHandler(t *testing.T) {
	varName1 := "john"
	varVal1 := "lenon"
	path1 := fmt.Sprintf("/rest/v1/env/%s", varName1)
	data := url.Values{}
	data.Set("val", varVal1)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "POST", path1, body)
	assert.Equal(t, http.StatusOK, rr.Code)

	varName2 := "ringo"
	varVal2 := "star"
	path2 := fmt.Sprintf("/rest/v1/env/%s", varName2)
	data = url.Values{}
	data.Set("val", varVal2)
	body = strings.NewReader(data.Encode())
	rr = sendRequest(t, "POST", path2, body)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())

	rr = sendRequest(t, "GET", path1, nil)
	assert.Equal(t, varVal1, rr.Body.String())
	rr = sendRequest(t, "GET", path2, nil)
	assert.Equal(t, varVal2, rr.Body.String())

	rr = sendRequest(t, "DELETE", path1, nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())
	rr = sendRequest(t, "GET", path1, nil)
	assert.Equal(t, http.StatusNotFound, rr.Code)

	rr = sendRequest(t, "GET", path2, nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, varVal2, rr.Body.String())

}

// func TestAppHandler(t *testing.T) {
// 	exe := "/bin/sleep"
// 	command := fmt.Sprintf("%s 3", exe)
// 	path := "/app/"
// 	data := url.Values{}
// 	data.Set("command", command)
// 	body := strings.NewReader(data.Encode())
// 	rr := sendRequest(t, "PUT", path, body)
// 	assert.Equal(t, http.StatusOK, rr.Code)
// 	pid, err := strconv.Atoi(rr.Body.String())
// 	assert.Nil(t, err)
// 	time.Sleep(500 * time.Millisecond)
// 	procfile := fmt.Sprintf("/proc/%d/cmdline", pid)
// 	assertFileHasContents(t, procfile, exe)
// }

// func TestAppHandlerEnv(t *testing.T) {
// 	varName := "THIS_IS_A_VERY_UNIQUE_VAR"
// 	varVal := "bar"
// 	path := fmt.Sprintf("/env/%s", varName)
// 	data := url.Values{}
// 	data.Set("val", varVal)
// 	body := strings.NewReader(data.Encode())
// 	rr := sendRequest(t, "POST", path, body)
// 	assert.Equal(t, http.StatusOK, rr.Code)

// 	exe := "/bin/sleep"
// 	command := fmt.Sprintf("%s 1", exe)
// 	path = "/app/"
// 	data = url.Values{}
// 	data.Set("command", command)
// 	body = strings.NewReader(data.Encode())
// 	rr = sendRequest(t, "PUT", path, body)
// 	assert.Equal(t, rr.Code, http.StatusOK)
// 	pid, err := strconv.Atoi(rr.Body.String())
// 	assert.Nil(t, err)
// 	time.Sleep(500 * time.Millisecond)

// 	procfile := fmt.Sprintf("/proc/%d/environ", pid)
// 	assertFileHasContents(t, procfile, fmt.Sprintf("%s=%s", varName, varVal))
// }

// generates a temporary filename for use in testing or whatever
// https://stackoverflow.com/questions/28005865/golang-generate-unique-filename-with-extension
func tempfileName(prefix, suffix string) string {
	randBytes := make([]byte, 6)
	_, _ = rand.Read(randBytes)
	return filepath.Join(os.TempDir(), prefix+hex.EncodeToString(randBytes)+suffix)
}

func deleteFile(path string) {
	_, err := os.Stat(path)
	if err == nil {
		_ = os.Remove(path)
	}
}

// TODO: once the logic has been moved to a separate package, mock out tosi.
// This test hits the actual official docker registry, and might take >30s.
func TestDeployImage(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "itzo-test-deploy-")
	assert.Nil(t, err)
	defer tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	rootdir, err := ioutil.TempDir("", "itzo-pkg-test")
	assert.Nil(t, err)
	srv := New(rootdir)
	srv.getHandlers()

	unit := fmt.Sprintf("alpine-%d", time.Now().UnixNano())

	data := url.Values{}
	data.Set("image", "library/alpine:latest")
	body := strings.NewReader(data.Encode())
	req, err := http.NewRequest("POST", "/rest/v1/deploy/"+unit, body)
	assert.Nil(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.getHandlers()
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	err = filepath.Walk(rootdir, func(path string, info os.FileInfo, e error) error {
		assert.Equal(t, info.Sys().(*syscall.Stat_t).Uid, uint32(os.Geteuid()))
		assert.Equal(t, info.Sys().(*syscall.Stat_t).Gid, uint32(os.Getegid()))
		return nil
	})
	assert.Equal(t, err, nil)

	rootfs := filepath.Join(rootdir, unit, "ROOTFS")
	fi, err := os.Stat(rootfs)
	assert.Nil(t, err)
	assert.NotNil(t, fi)
	isempty, err := isEmptyDir(rootfs)
	assert.False(t, isempty)
	assert.Nil(t, err)
}

func TestDeployImageFail(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "itzo-test-deploy-")
	assert.Nil(t, err)
	defer tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	rootdir, err := ioutil.TempDir("", "itzo-pkg-test")
	assert.Nil(t, err)
	srv := New(rootdir)
	srv.getHandlers()

	unit := fmt.Sprintf("alpine-%d", time.Now().UnixNano())

	data := url.Values{}
	data.Set("image", "library/this-container-image-does-not-exist:latest")
	body := strings.NewReader(data.Encode())
	req, err := http.NewRequest("POST", "/rest/v1/deploy/"+unit, body)
	assert.Nil(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.getHandlers()
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestGetLogs(t *testing.T) {
	command := "echo foobar"
	path := "/rest/v1/start/echo"
	data := url.Values{}
	data.Set("command", command)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "PUT", path, body)
	assert.Equal(t, rr.Code, http.StatusOK)
	var lines []string
	timeout := time.Now().Add(3 * time.Second)
	for time.Now().Before(timeout) {
		path = "/rest/v1/logs/echo"
		rr = sendRequest(t, "GET", path, nil)
		assert.Equal(t, rr.Code, http.StatusOK)
		lines = strings.Split(rr.Body.String(), "\n")
		if len(lines) >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.True(t, 2 <= len(lines))
	found := false
	for _, line := range lines {
		if strings.Contains(line, "foobar") {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestGetLogsLines(t *testing.T) {
	command := "sh -c \"yes|head -n10\""
	path := "/rest/v1/start/yes"
	data := url.Values{}
	data.Set("command", command)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "PUT", path, body)
	assert.Equal(t, rr.Code, http.StatusOK)
	var lines []string
	timeout := time.Now().Add(3 * time.Second)
	for time.Now().Before(timeout) {
		path = "/rest/v1/logs/yes?lines=3"
		rr = sendRequest(t, "GET", path, nil)
		assert.Equal(t, rr.Code, http.StatusOK)
		lines = strings.Split(rr.Body.String(), "\n")
		if len(lines) >= 4 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.True(t, 4 <= len(lines))
	for _, line := range lines[:len(lines)-1] {
		assert.Equal(t, line, "y")
	}
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

func TestStart(t *testing.T) {
	rnd := randStr(t, 16)
	command := fmt.Sprintf("echo %s", rnd)
	path := fmt.Sprintf("/rest/v1/start/%s", rnd)
	data := url.Values{}
	data.Set("command", command)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "PUT", path, body)
	assert.Equal(t, rr.Code, http.StatusOK)
	pid, err := strconv.Atoi(rr.Body.String())
	assert.Nil(t, err)
	assert.True(t, pid > 0)
}

func TestStatusHandler(t *testing.T) {
	rnd := randStr(t, 16)
	command := fmt.Sprintf("echo %s", rnd)
	path := fmt.Sprintf("/rest/v1/start/%s", rnd)
	data := url.Values{}
	data.Set("command", command)
	data.Set("restartpolicy", "always")
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "PUT", path, body)
	assert.Equal(t, rr.Code, http.StatusOK)
	pid, err := strconv.Atoi(rr.Body.String())
	assert.Nil(t, err)
	assert.True(t, pid > 0)
	path = fmt.Sprintf("/rest/v1/status/%s", rnd)
	data = url.Values{}
	body = strings.NewReader(data.Encode())
	status := ""
	timeout := time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		rr = sendRequest(t, "GET", path, body)
		assert.Equal(t, rr.Code, http.StatusOK)
		status = rr.Body.String()
		if status == "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, "running", status)
}

func TestStatusHandlerStartFailed(t *testing.T) {
	rnd := randStr(t, 16)
	command := "/does_not_exist"
	path := fmt.Sprintf("/rest/v1/start/%s", rnd)
	data := url.Values{}
	data.Set("command", command)
	data.Set("restartpolicy", "never")
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "PUT", path, body)
	assert.Equal(t, rr.Code, http.StatusOK)
	pid, err := strconv.Atoi(rr.Body.String())
	assert.Nil(t, err)
	assert.True(t, pid > 0)
	path = fmt.Sprintf("/rest/v1/status/%s", rnd)
	data = url.Values{}
	body = strings.NewReader(data.Encode())
	status := ""
	timeout := time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		rr = sendRequest(t, "GET", path, body)
		assert.Equal(t, rr.Code, http.StatusOK)
		status = rr.Body.String()
		if status == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, "failed", status)
}

func TestStatusHandlerFailed(t *testing.T) {
	rnd := randStr(t, 16)
	command := "cat /does_not_exist"
	path := fmt.Sprintf("/rest/v1/start/%s", rnd)
	data := url.Values{}
	data.Set("command", command)
	data.Set("restartpolicy", "never")
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "PUT", path, body)
	assert.Equal(t, rr.Code, http.StatusOK)
	pid, err := strconv.Atoi(rr.Body.String())
	assert.Nil(t, err)
	assert.True(t, pid > 0)
	path = fmt.Sprintf("/rest/v1/status/%s", rnd)
	data = url.Values{}
	body = strings.NewReader(data.Encode())
	status := ""
	timeout := time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		rr = sendRequest(t, "GET", path, body)
		assert.Equal(t, rr.Code, http.StatusOK)
		status = rr.Body.String()
		if status == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, "failed", status)
}

func TestStatusHandlerSucceeded(t *testing.T) {
	rnd := randStr(t, 16)
	command := fmt.Sprintf("echo %s", rnd)
	path := fmt.Sprintf("/rest/v1/start/%s", rnd)
	data := url.Values{}
	data.Set("command", command)
	data.Set("restartpolicy", "never")
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "PUT", path, body)
	assert.Equal(t, rr.Code, http.StatusOK)
	pid, err := strconv.Atoi(rr.Body.String())
	assert.Nil(t, err)
	assert.True(t, pid > 0)
	path = fmt.Sprintf("/rest/v1/status/%s", rnd)
	data = url.Values{}
	body = strings.NewReader(data.Encode())
	status := ""
	timeout := time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		rr = sendRequest(t, "GET", path, body)
		assert.Equal(t, rr.Code, http.StatusOK)
		status = rr.Body.String()
		if status == "succeeded" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, "succeeded", status)
}
