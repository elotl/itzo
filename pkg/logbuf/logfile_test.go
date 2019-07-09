package logbuf

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	logFileName = "myLog"
	sampleLine  = []byte("This is my log line\n")
)

func newTestDir(t *testing.T) (string, func()) {
	tempDir, err := ioutil.TempDir("", "logfileTest")
	if err != nil {
		t.FailNow()
	}
	closer := func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.FailNow()
		}
	}
	return tempDir, closer
}

func newTestRotator(t *testing.T, size int) (*RotatingFile, func()) {
	tempDir, closer := newTestDir(t)
	rf, err := NewRotatingFile(tempDir, logFileName, size)
	if err != nil {
		t.FailNow()
	}
	return rf, closer
}

func assertFileAbsent(t *testing.T, filepath string) {
	_, err := os.Stat(filepath)
	if !os.IsNotExist(err) {
		t.Fail()
	}
}

func assertFileContains(t *testing.T, filepath string, contents []byte) {
	b, err := ioutil.ReadFile(filepath)
	assert.NoError(t, err)
	assert.Equal(t, contents, b)
}

func TestRotatingFileWrite(t *testing.T) {
	rot, closer := newTestRotator(t, 100000)
	defer closer()

	_, err := rot.Write(sampleLine)
	assert.NoError(t, err)
	assert.FileExists(t, rot.filePath())
	assertFileAbsent(t, rot.rotatedFilePath())
	assertFileContains(t, rot.filePath(), sampleLine)
}

func TestRotatingFileRotate(t *testing.T) {
	rot, closer := newTestRotator(t, 100000)
	defer closer()
	_, err := rot.Write(sampleLine)
	assert.NoError(t, err)
	rot.rotate()
	assert.FileExists(t, rot.filePath())
	assert.FileExists(t, rot.rotatedFilePath())
}

func TestRotatingFileWriteRotates(t *testing.T) {
	rot, closer := newTestRotator(t, 50)
	defer closer()
	line := []byte("this is a really long line with a lot to say, we should rotate after writing this line")
	_, err := rot.Write(line)
	assert.NoError(t, err)
	assert.FileExists(t, rot.filePath())
	assert.FileExists(t, rot.rotatedFilePath())
	assertFileContains(t, rot.rotatedFilePath(), line)
	assertFileContains(t, rot.filePath(), []byte(""))
}

func TestLogOutputWrite(t *testing.T) {
	tempDir, closer := newTestDir(t)
	defer closer()
	emitter, err := NewLogOutput(tempDir, logFileName, 100000)
	assert.NoError(t, err)
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	entry := LogEntry{
		Timestamp: nowStr,
		Source:    StdoutLogSource,
		Line:      string(sampleLine),
	}
	emitter.Write(entry)
	filepath := path.Join(tempDir, logFileName)
	b, err := ioutil.ReadFile(filepath)
	assert.NoError(t, err)
	bytes.Contains(b, sampleLine)
	bytes.Contains(b, []byte(nowStr))
}
