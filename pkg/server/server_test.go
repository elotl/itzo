package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func sendRequest(t *testing.T, method, url string, body io.Reader, r *mux.Router) *httptest.ResponseRecorder {
	req, err := http.NewRequest(method, url, body)
	assert.Nil(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	// I'm a bit sad about how testing gorilla/mux handlers turned out
	// you need to pass around the router
	r.ServeHTTP(rr, req)
	return rr
}

func TestHealthcheckHandler(t *testing.T) {
	s := Server{}
	r := s.getHandlers()
	rr := sendRequest(t, "GET", "/milpa/health", nil, r) //, s.healthcheckHandler)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())
}

func TestEnvHandler(t *testing.T) {
	s := Server{env: StringMap{data: map[string]string{}}}
	r := s.getHandlers()
	varName1 := "john"
	varVal1 := "lenon"
	path1 := fmt.Sprintf("/env/%s", varName1)
	data := url.Values{}
	data.Set("val", varVal1)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "POST", path1, body, r)
	assert.Equal(t, http.StatusOK, rr.Code)

	varName2 := "ringo"
	varVal2 := "star"
	path2 := fmt.Sprintf("/env/%s", varName2)
	data = url.Values{}
	data.Set("val", varVal2)
	body = strings.NewReader(data.Encode())
	rr = sendRequest(t, "POST", path2, body, r)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())

	rr = sendRequest(t, "GET", path1, nil, r)
	assert.Equal(t, varVal1, rr.Body.String())
	rr = sendRequest(t, "GET", path2, nil, r)
	assert.Equal(t, varVal2, rr.Body.String())

	rr = sendRequest(t, "DELETE", path1, nil, r)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())
	rr = sendRequest(t, "GET", path1, nil, r)
	assert.Equal(t, http.StatusNotFound, rr.Code)

	rr = sendRequest(t, "GET", path2, nil, r)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, varVal2, rr.Body.String())

}

func TestAppHandler(t *testing.T) {
	s := Server{env: StringMap{data: map[string]string{}}}
	r := s.getHandlers()
	exe := "/bin/sleep"
	command := fmt.Sprintf("%s 3", exe)
	path := "/app"
	data := url.Values{}
	data.Set("command", command)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "PUT", path, body, r)
	assert.Equal(t, http.StatusOK, rr.Code)
	pid, err := strconv.Atoi(rr.Body.String())
	assert.Nil(t, err)
	time.Sleep(500 * time.Millisecond)
	procfile := fmt.Sprintf("/proc/%d/cmdline", pid)
	f, err := os.Open(procfile)
	assert.Nil(t, err)
	content := make([]byte, 255)
	_, err = f.Read(content)
	assert.Nil(t, err)
	cmdline := string(content[:]) // This ends up looking like
	assert.True(t, strings.HasPrefix(cmdline, exe))
}

func TestAppHandlerEnv(t *testing.T) {
	s := Server{env: StringMap{data: map[string]string{}}}
	r := s.getHandlers()
	varName := "THIS_IS_A_VERY_UNIQUE_VAR"
	varVal := "bar"
	path := fmt.Sprintf("/env/%s", varName)
	data := url.Values{}
	data.Set("val", varVal)
	body := strings.NewReader(data.Encode())
	rr := sendRequest(t, "POST", path, body, r)
	assert.Equal(t, http.StatusOK, rr.Code)

	exe := "/bin/sleep"
	command := fmt.Sprintf("%s 3", exe)
	path = "/app"
	data = url.Values{}
	data.Set("command", command)
	body = strings.NewReader(data.Encode())
	rr = sendRequest(t, "PUT", path, body, r)
	assert.Equal(t, rr.Code, http.StatusOK)
	pid, err := strconv.Atoi(rr.Body.String())
	assert.Nil(t, err)
	time.Sleep(500 * time.Millisecond)

	procfile := fmt.Sprintf("/proc/%d/environ", pid)
	f, err := os.Open(procfile)
	assert.Nil(t, err)
	content := make([]byte, 10000)
	_, err = f.Read(content)
	assert.Nil(t, err)
	envVars := string(content[:]) // This ends up looking like
	assert.Contains(t, envVars, fmt.Sprintf("%s=%s", varName, varVal))
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
	s := Server{env: StringMap{data: map[string]string{}}}
	r := s.getHandlers()

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
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	// make sure the file exists
	_, err = os.Stat(temppath)
	assert.Nil(t, err)
	// make sure it has the contents

}
