package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	stdlibpath "path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var s Server

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags
	var appcmdline = flag.String("exec", "", "Command for starting a unit")
	flag.Parse()
	if *appcmdline != "" {
		StartUnit("", strings.Split(*appcmdline, " "))
		os.Exit(0)
	}
	s = Server{env: StringMap{data: map[string]string{}}}
	s.getHandlers()
	os.Exit(m.Run())
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

func TestHealthcheckHandler(t *testing.T) {
	rr := sendRequest(t, "GET", "/milpa/health", nil) //, s.healthcheckHandler)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())
}

func TestEnvHandler(t *testing.T) {
	varName1 := "john"
	varVal1 := "lenon"
	path1 := fmt.Sprintf("/env/%s", varName1)
	data := url.Values{}
	data.Set("val", varVal1)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "POST", path1, body)
	assert.Equal(t, http.StatusOK, rr.Code)

	varName2 := "ringo"
	varVal2 := "star"
	path2 := fmt.Sprintf("/env/%s", varName2)
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

func TestAppHandler(t *testing.T) {
	exe := "/bin/sleep"
	command := fmt.Sprintf("%s 3", exe)
	path := "/app/"
	data := url.Values{}
	data.Set("command", command)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "PUT", path, body)
	assert.Equal(t, http.StatusOK, rr.Code)
	pid, err := strconv.Atoi(rr.Body.String())
	assert.Nil(t, err)
	time.Sleep(500 * time.Millisecond)
	procfile := fmt.Sprintf("/proc/%d/cmdline", pid)
	assertFileHasContents(t, procfile, exe)
}

func TestAppHandlerEnv(t *testing.T) {
	varName := "THIS_IS_A_VERY_UNIQUE_VAR"
	varVal := "bar"
	path := fmt.Sprintf("/env/%s", varName)
	data := url.Values{}
	data.Set("val", varVal)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "POST", path, body)
	assert.Equal(t, http.StatusOK, rr.Code)

	exe := "/bin/sleep"
	command := fmt.Sprintf("%s 1", exe)
	path = "/app/"
	data = url.Values{}
	data.Set("command", command)
	body = strings.NewReader(data.Encode())
	rr = sendRequest(t, "PUT", path, body)
	assert.Equal(t, rr.Code, http.StatusOK)
	pid, err := strconv.Atoi(rr.Body.String())
	assert.Nil(t, err)
	time.Sleep(500 * time.Millisecond)

	procfile := fmt.Sprintf("/proc/%d/environ", pid)
	assertFileHasContents(t, procfile, fmt.Sprintf("%s=%s", varName, varVal))
}

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

func TestFileUploader(t *testing.T) {
	temppath := tempfileName("ServerTest", "")
	defer deleteFile(temppath)
	url := fmt.Sprintf("/file/%s", url.PathEscape(temppath))
	content := fmt.Sprintf("The time at the tone is %s... BEEP!", time.Now().String())
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(MULTIPART_FILE_NAME, temppath)
	assert.Nil(t, err)
	_, err = part.Write([]byte(content))
	assert.Nil(t, err)
	err = writer.Close()
	assert.Nil(t, err)

	req, err := http.NewRequest("POST", url, body)
	assert.Nil(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	_, err = os.Stat(temppath)
	assert.Nil(t, err)
	assertFileHasContents(t, temppath, string(content))
}

func createTarGzBuf(t *testing.T, rootdir, unit string) []byte {
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
		{"ROOTFS/hard.link", tar.TypeLink, "", fmt.Sprintf("%s/bin/data.bin", getRootfs(rootdir, unit)), 0660, uid, gid},
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

func deployPackage(t *testing.T, unit string) {
	tmpfile, err := ioutil.TempFile("", "itzo-test-deploy-")
	assert.Nil(t, err)
	defer tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	rootdir, err := ioutil.TempDir("", "milpa-pkg-test")
	assert.Nil(t, err)

	srv := New(rootdir)
	srv.getHandlers()

	// Create a .tar.gz file.
	content := createTarGzBuf(t, srv.installRootdir, unit)
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(MULTIPART_PKG_NAME, tmpfile.Name())
	assert.Nil(t, err)
	_, err = part.Write(content)
	assert.Nil(t, err)
	err = writer.Close()
	assert.Nil(t, err)

	path := "/milpa/deploy"
	if unit != "" {
		path = stdlibpath.Join(path, unit)
	}
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

func TestDeployPackage(t *testing.T) {
	deployPackage(t, "")
}

func TestDeployPackageWithUnit(t *testing.T) {
	deployPackage(t, "foobar")
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
	part, err := writer.CreateFormFile(MULTIPART_PKG_NAME, tmpfile.Name())
	assert.Nil(t, err)
	_, err = part.Write(content)
	assert.Nil(t, err)
	err = writer.Close()
	assert.Nil(t, err)

	req, err := http.NewRequest("POST", "/milpa/deploy", body)
	assert.Nil(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	srv := New("/tmp/milpa-pkg-test")
	srv.getHandlers()
	srv.ServeHTTP(rr, req)

	assert.NotEqual(t, http.StatusOK, rr.Code)
}

func TestStart(t *testing.T) {
	exe := "/bin/echo"
	command := fmt.Sprintf("%s %s", exe, "foobar")
	path := "/milpa/start/foobar"
	data := url.Values{}
	data.Set("command", command)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "PUT", path, body)
	assert.Equal(t, rr.Code, http.StatusOK)
	pid, err := strconv.Atoi(rr.Body.String())
	assert.Nil(t, err)
	assert.True(t, pid > 0)
}
