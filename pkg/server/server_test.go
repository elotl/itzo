package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net"
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

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/wsstream"
	"github.com/gorilla/websocket"
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
	var workingdir = flag.String("workingdir", "", "Working directory")
	var rp = flag.String("restartpolicy", string(api.RestartPolicyAlways), "Restart policy")
	flag.Parse()
	if *appcmdline != "" {
		policy := api.RestartPolicy(*rp)
		StartUnit(*rootdir, *unit, *workingdir, strings.Split(*appcmdline, " "), policy)
		os.Exit(0)
	}
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	if err != nil {
		panic("Error creating temporary directory")
	}
	defer os.RemoveAll(tmpdir)

	s = Server{
		env:            EnvStore{},
		installRootdir: tmpdir,
		podController:  NewPodController(tmpdir, nil, nil),
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
		Command: []string{"echo", "Hello Milpa"},
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
		Command: []string{"echo", "Hello World"},
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
	timeout := time.Now().Add(30 * time.Second)
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
			Command: []string{"ls", "/does_not_exist"},
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
			Command: []string{"/does_not_exist"},
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
			Command: []string{"/does_not_exist"},
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

func TestGetLogsFunctional(t *testing.T) {
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

func TestGetLogsLinesFunctional(t *testing.T) {
	if !*runFunctional {
		return
	}
	params := createUnit(t)
	params.Spec.Units[0].Command = []string{"sh", "-c", "yes | head -n10"}
	buf, err := json.Marshal(params)
	assert.NoError(t, err)
	body := strings.NewReader(string(buf))
	rr := sendRequest(t, "POST", "/rest/v1/updatepod", body)
	assert.Equal(t, http.StatusOK, rr.Code)
	var lines []string
	timeout := time.Now().Add(3 * time.Second)
	for time.Now().Before(timeout) {
		path := "/rest/v1/logs/yes?lines=3&bytes=0"
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

func createTarGzBuf(t *testing.T) []byte {
	var uid int = os.Geteuid()
	var gid int = os.Getegid()
	var entries = []struct {
		Name       string
		Type       byte
		Body       string
		LinkTarget string
		Mode       int64
		Uid        int
		Gid        int
	}{
		{"ROOTFS/", tar.TypeDir, "", "", 0755, uid, gid},
		{"ROOTFS/bin", tar.TypeDir, "", "", 0700, uid, gid},
		{"ROOTFS/readme.link", tar.TypeSymlink, "", "./readme.txt", 0000, uid, gid},
		{"ROOTFS/readme.txt", tar.TypeReg, "This is a textfile.", "", 0640, uid, gid},
		{"ROOTFS/bin/data.bin", tar.TypeReg, string([]byte{0x11, 0x22, 0x33, 0x44}), "", 0600, uid, gid},
	}

	// Create a tar buffer in memory.
	tarbuf := new(bytes.Buffer)
	tw := tar.NewWriter(tarbuf)
	for _, entry := range entries {
		hdr := &tar.Header{
			Name:     entry.Name,
			Mode:     entry.Mode,
			Size:     int64(len(entry.Body)),
			Typeflag: entry.Type,
			Linkname: entry.LinkTarget,
			Uid:      entry.Uid,
			Gid:      entry.Gid,
		}
		err := tw.WriteHeader(hdr)
		assert.Nil(t, err)
		_, err = tw.Write([]byte(entry.Body))
		assert.Nil(t, err)
	}
	err := tw.Close()
	assert.Nil(t, err)

	// Create our gzip buffer, effectively a .tar.gz in memory.
	var gzbuf bytes.Buffer
	zw := gzip.NewWriter(&gzbuf)
	_, err = zw.Write(tarbuf.Bytes())
	assert.Nil(t, err)
	zw.Close()
	assert.Nil(t, err)

	return gzbuf.Bytes()
}

func TestDeployPackage(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "itzo-test-deploy-")
	assert.Nil(t, err)
	defer tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	rootdir, err := ioutil.TempDir("", "itzo-pkg-test")
	assert.Nil(t, err)

	srv := New(rootdir)
	srv.getHandlers()

	content := createTarGzBuf(t)
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(MULTIPART_PACKAGE, tmpfile.Name())
	assert.Nil(t, err)
	_, err = part.Write(content)
	assert.Nil(t, err)
	err = writer.Close()
	assert.Nil(t, err)

	path := "/rest/v1/deploy/mypod/pkg111"
	req, err := http.NewRequest("POST", path, body)
	assert.Nil(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	err = filepath.Walk(rootdir, func(path string, info os.FileInfo, e error) error {
		assert.Equal(t, info.Sys().(*syscall.Stat_t).Uid, uint32(os.Geteuid()))
		assert.Equal(t, info.Sys().(*syscall.Stat_t).Gid, uint32(os.Getegid()))
		return nil
	})
	assert.Equal(t, err, nil)
}

func TestDeployInvalidPackage(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "itzo-test-deploy-")
	assert.Nil(t, err)
	defer tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	// Create an invalid .tar.gz file.
	content := []byte{0xde, 0xad, 0xbe, 0xef}
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(MULTIPART_PACKAGE, tmpfile.Name())
	assert.Nil(t, err)
	_, err = part.Write(content)
	assert.Nil(t, err)
	err = writer.Close()
	assert.Nil(t, err)

	req, err := http.NewRequest("POST", "/rest/v1/deploy/mypod/pkg222", body)
	assert.Nil(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	srv := New("/tmp/itzo-pkg-test")
	srv.getHandlers()
	srv.ServeHTTP(rr, req)

	assert.NotEqual(t, http.StatusOK, rr.Code)
}

func TestGetLogs(t *testing.T) {
	unitName := "testunit"
	um := NewUnitManager(DEFAULT_ROOTDIR)
	s.unitMgr = um
	lb := logbuf.NewLogBuffer(1000)
	um.logbuf.Set(unitName, lb)
	for i := 0; i < 10; i++ {
		lb.Write("somesource", fmt.Sprintf("%d\n", i))
	}
	nLines := 5
	path := fmt.Sprintf("/rest/v1/logs/%s?lines=%d&bytes=0", unitName, nLines)
	rr := sendRequest(t, "GET", path, strings.NewReader(""))
	assert.Equal(t, http.StatusOK, rr.Code)
	responseBody := rr.Body.String()
	if strings.HasSuffix(responseBody, "\n") {
		responseBody = responseBody[:len(responseBody)-1]
	}
	lines := strings.Split(responseBody, "\n")
	assert.Equal(t, []string{"5", "6", "7", "8", "9"}, lines)
}

func runServer() (*Server, func(), int) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	if err != nil {
		panic("Error creating temporary directory")
	}
	closer := func() { os.RemoveAll(tmpdir) }
	s := &Server{
		installRootdir: tmpdir,
		unitMgr:        NewUnitManager(tmpdir),
		podController:  NewPodController(tmpdir, nil, nil),
	}
	s.getHandlers()
	s.httpServer = &http.Server{Addr: ":0", Handler: s}
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	go s.httpServer.Serve(listener)
	return s, closer, port
}

func createWebsocketClient(port, path string) (*wsstream.WSStream, error) {
	addr := ":" + port
	u := url.URL{Scheme: "ws", Host: addr, Path: path}
	header := http.Header{}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		return nil, err
	}
	return wsstream.NewWSStream(c), nil
}

func TestPortForward(t *testing.T) {
	// We start up our server, start a websocket port forwarwd request
	// to the same server port and then forward, throught the
	// websocket, a request to ping we expect pong as the output
	ss, closer, port := runServer()
	defer closer()
	portstr := fmt.Sprintf("%d", port)
	time.Sleep(1 * time.Second)
	defer ss.httpServer.Close()

	ws, err := createWebsocketClient(portstr, "/rest/v1/portforward/")
	assert.NoError(t, err)

	pfp := api.PortForwardParams{
		Port: portstr,
	}
	pfpb, err := json.Marshal(pfp)
	assert.NoError(t, err)
	err = ws.WriteRaw(pfpb)
	assert.NoError(t, err)
	msg := []byte("GET /rest/v1/ping HTTP/1.1\nHost: localhost:" + portstr + "\r\n\r\n")
	err = ws.WriteMsg(0, msg)
	assert.NoError(t, err)
	timeout := 3 * time.Second
	select {
	case f := <-ws.ReadMsg():
		c, m, err := wsstream.UnpackMessage(f)
		assert.NoError(t, err)
		assert.Equal(t, 1, c)
		assert.True(t, strings.HasSuffix(string(m), "pong"))
	case <-time.After(timeout):
		assert.FailNow(t, "reading timed out")
	}
}

func TestExec(t *testing.T) {
	// We start up our server, start an exec request
	ss, closer, port := runServer()
	defer closer()
	portstr := fmt.Sprintf("%d", port)
	time.Sleep(1 * time.Second)
	defer ss.httpServer.Close()

	unitName := "testunit"
	ss.podController.podStatus.Units = []api.Unit{{
		Name: unitName,
	}}

	ws, err := createWebsocketClient(portstr, "/rest/v1/exec/")
	assert.NoError(t, err)

	params := api.ExecParams{
		Command:     []string{"/bin/cat", "/proc/version"},
		Interactive: false,
		TTY:         false,
		SkipNSEnter: true,
	}
	paramsb, err := json.Marshal(params)
	assert.NoError(t, err)
	err = ws.WriteRaw(paramsb)
	assert.NoError(t, err)
	out := <-ws.ReadMsg()

	c, msg, err := wsstream.UnpackMessage(out)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(msg), "Linux"))
	assert.Equal(t, 1, c)

	exit := <-ws.ReadMsg()
	c, msg, err = wsstream.UnpackMessage(exit)
	assert.NoError(t, err)
	assert.Equal(t, "0", string(msg))
	assert.Equal(t, 3, c)
}

// Todo: This test is a gosh darn tragedy...  It's closer to an
// end-to-end test that makes use of the unitMg logs, pod controller,
// unit and unit pipes as well as the server.  :( It's going to be a
// change detector test If this gets in the way, comment it out,
// assign an issue to bcox.
func TestAttach(t *testing.T) {
	unitName := "testunit"
	ss, closer, port := runServer()
	defer closer()
	portstr := fmt.Sprintf("%d", port)
	defer ss.httpServer.Close()
	// need the pod controller in order to get the unit
	ss.podController.podStatus.Units = []api.Unit{{
		Name: unitName,
	}}

	// Open the unit
	u, err := OpenUnit(ss.installRootdir, unitName)
	assert.NoError(t, err)
	defer u.Destroy()

	ss.unitMgr.CaptureLogs(unitName, u)
	// silly hack that allows us to get the output from the unit
	ss.unitMgr.runningUnits.Set(unitName, nil)
	unitin, err := u.openStdinReader()
	assert.NoError(t, err)
	lp := u.LogPipe
	unitout, err := lp.OpenWriter(PIPE_UNIT_STDOUT, false)
	defer unitout.Close()

	// start a unit that we can get stdin and stdout from
	ch := make(chan error)
	go func() {
		err = u.runUnitLoop(
			[]string{"/bin/cat", "-"},
			[]string{}, unitin, unitout, nil, api.RestartPolicyNever)
		ch <- err
	}()

	ws, err := createWebsocketClient(portstr, "/rest/v1/attach/")
	assert.NoError(t, err)
	defer ws.CloseAndCleanup()

	params := api.AttachParams{
		Interactive: true,
	}
	paramsb, err := json.Marshal(params)
	assert.NoError(t, err)
	err = ws.WriteRaw(paramsb)
	assert.NoError(t, err)

	msgString := []byte("Hello Milpa\n") // don't forget newline, we are line based
	err = ws.WriteMsg(wsstream.StdinChan, msgString)
	assert.NoError(t, err)

	timeout := 3 * time.Second
	select {
	case f := <-ws.ReadMsg():
		c, m, err := wsstream.UnpackMessage(f)
		assert.NoError(t, err)
		assert.Equal(t, 1, c)
		assert.Equal(t, msgString, m)
	case <-time.After(timeout):
		assert.FailNow(t, "reading timed out")
	}
}

func TestRunCmd(t *testing.T) {
	cmdParams := api.RunCmdParams{
		Command: []string{"/bin/cat", "/proc/cpuinfo"},
	}
	b, err := json.Marshal(cmdParams)
	assert.NoError(t, err)
	buf := bytes.NewBuffer(b)
	rr := sendRequest(t, "GET", "/rest/v1/runcmd/", buf)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, strings.Contains(rr.Body.String(), "processor"))

	cmdParams.Command = []string{"/this/command/isnt/found"}
	b, err = json.Marshal(cmdParams)
	assert.NoError(t, err)
	buf = bytes.NewBuffer(b)
	rr = sendRequest(t, "GET", "/rest/v1/runcmd/", buf)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
