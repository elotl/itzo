package server

import (
	"bytes"
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
	path1 := "/rest/v1/env/foounit/" + varName1
	data := url.Values{}
	data.Set("val", varVal1)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "POST", path1, body)
	assert.Equal(t, http.StatusOK, rr.Code)

	varName2 := "ringo"
	varVal2 := "star"
	path2 := "/rest/v1/env/foounit/" + varName2
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

func TestDeployImage(t *testing.T) {
	if !(*runFunctional) {
		// Mock out tosi.
		TOSI_PRG = "true"
	}
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

	if !(*runFunctional) {
		return
	}

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
	if !(*runFunctional) {
		// Mock out tosi, don't try to access Docker Hub.
		TOSI_PRG = "false"
	}
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

func TestGetLogsLines(t *testing.T) {
	command := "sh -c yes | head -n10"
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
		assert.Equal(t, "y", line)
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

func TestCreateMountHandler(t *testing.T) {
	defer os.RemoveAll(filepath.Join(s.installRootdir, "../mounts"))
	path := fmt.Sprintf("/rest/v1/mount")
	vol := `{
        "name": "test-mount",
        "emptyDir": {}
    }`
	body := bytes.NewBuffer([]byte(vol))
	rr := sendRequest(t, "POST", path, body)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestCreateMountHandlerFail(t *testing.T) {
	//mounter := func(source, target, fstype string, flags uintptr, data string) error {
	// 	return fmt.Errorf("Test /mount failure")
	// }
	defer os.RemoveAll(filepath.Join(s.installRootdir, "../mounts"))
	path := fmt.Sprintf("/rest/v1/mount")
	vol := `{
        "name": "test-mount",
        "emptyDir": {
            "medium": "Memory"
        }
    }`
	body := bytes.NewBuffer([]byte(vol))
	rr := sendRequest(t, "POST", path, body)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestCreateMountHandlerUserError(t *testing.T) {
	defer os.RemoveAll(filepath.Join(s.installRootdir, "../mounts"))
	path := fmt.Sprintf("/rest/v1/mount")
	vol := ""
	body := bytes.NewBuffer([]byte(vol))
	rr := sendRequest(t, "POST", path, body)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestAttachMountHandler(t *testing.T) {
	//mounter := func(source, target, fstype string, flags uintptr, data string) error {
	// 	return nil
	// }
	defer os.RemoveAll(s.installRootdir)
	defer os.RemoveAll(filepath.Join(s.installRootdir, "../mounts"))
	err := os.MkdirAll(filepath.Join(s.installRootdir, "my-unit", "ROOTFS"), 0755)
	assert.Nil(t, err)
	err = os.MkdirAll(filepath.Join(s.installRootdir, "../mounts", "my-mount"), 0755)
	assert.Nil(t, err)
	path := fmt.Sprintf("/rest/v1/mount/my-unit")
	data := url.Values{}
	data.Set("name", "my-mount")
	data.Set("path", "/mount-target")
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "POST", path, body)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestAttachMountHandlerFail(t *testing.T) {
	//mounter := func(source, target, fstype string, flags uintptr, data string) error {
	// 	return fmt.Errorf("Testing mount attach failure")
	// }
	defer os.RemoveAll(s.installRootdir)
	defer os.RemoveAll(filepath.Join(s.installRootdir, "../mounts"))
	err := os.MkdirAll(filepath.Join(s.installRootdir, "my-unit", "ROOTFS"), 0755)
	assert.Nil(t, err)
	err = os.MkdirAll(filepath.Join(s.installRootdir, "../mounts", "my-mount"), 0755)
	assert.Nil(t, err)
	path := fmt.Sprintf("/rest/v1/mount/my-unit")
	data := url.Values{}
	data.Set("name", "my-mount")
	data.Set("path", "/mount-target")
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "POST", path, body)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestAttachMountHandlerMissingMount(t *testing.T) {
	//mounter := func(source, target, fstype string, flags uintptr, data string) error {
	// 	return nil
	// }
	defer os.RemoveAll(s.installRootdir)
	defer os.RemoveAll(filepath.Join(s.installRootdir, "../mounts"))
	err := os.MkdirAll(filepath.Join(s.installRootdir, "my-unit", "ROOTFS"), 0755)
	assert.Nil(t, err)
	os.RemoveAll(filepath.Join(s.installRootdir, "../mounts", "my-mount"))
	path := fmt.Sprintf("/rest/v1/mount/my-unit")
	data := url.Values{}
	data.Set("name", "my-mount")
	data.Set("path", "/mount-target")
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "POST", path, body)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
