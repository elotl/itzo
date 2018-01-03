package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckName(t *testing.T) {
	checkName(PIPE_UNIT_STDOUT)
	checkName(PIPE_UNIT_STDERR)
	checkName(PIPE_HELPER_OUT)
}

func TestReadWrite(t *testing.T) {
	dir, err := ioutil.TempDir("", "pipe-test")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)
	lp, err := NewLogPipe(dir)
	assert.Nil(t, err)
	for _, name := range UNIT_PIPES {
		var buf bytes.Buffer
		var wg sync.WaitGroup
		wg.Add(1)
		lp.StartReader(name, func(line string) {
			defer wg.Done()
			buf.Write([]byte(line))
		})
		w, err := lp.OpenWriter(name, false)
		assert.Nil(t, err)
		defer w.Close()
		output := []byte(fmt.Sprintf("Hello %s!\n", name))
		w.Write(output)
		wg.Wait()
		assert.Equal(t, buf.Bytes(), output)
	}
}

func TestWriterClose(t *testing.T) {
	dir, err := ioutil.TempDir("", "pipe-test")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)
	lp, err := NewLogPipe(dir)
	assert.Nil(t, err)
	lp.StartAllReaders(func(line string) {
		fmt.Print(line)
	})
	for _, name := range UNIT_PIPES {
		assert.Nil(t, lp.Pipes[name])
		w, err := lp.OpenWriter(name, false)
		assert.Nil(t, err)
		assert.NotNil(t, w)
		assert.NotNil(t, lp.Pipes[name])
		assert.Equal(t, lp.Pipes[name], w)
		w.Close()
	}
	lp.Remove()
}

func TestRemoveInactive(t *testing.T) {
	dir, err := ioutil.TempDir("", "pipe-test")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)
	lp, err := NewLogPipe(dir)
	assert.Nil(t, err)
	lp.Remove()
	for _, name := range UNIT_PIPES {
		assert.Nil(t, lp.Pipes[name])
	}
	empty, err := isEmptyDir(dir)
	assert.True(t, empty)
}

func TestRemoveActive(t *testing.T) {
	dir, err := ioutil.TempDir("", "pipe-test")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)
	lp, err := NewLogPipe(dir)
	assert.Nil(t, err)
	lp.StartAllReaders(func(line string) {
		fmt.Print(line)
	})
	pipes := make(map[string]*os.File)
	for _, name := range UNIT_PIPES {
		assert.Nil(t, lp.Pipes[name])
		w, err := lp.OpenWriter(name, false)
		assert.Nil(t, err)
		assert.NotNil(t, w)
		pipes[name] = w
	}
	// Remove() will close any open pipes, and remove the fifos from the
	// filesystem.
	lp.Remove()
	for _, pipe := range pipes {
		_, err := pipe.Write([]byte("foobar"))
		assert.NotNil(t, err)
	}
	empty, err := isEmptyDir(dir)
	assert.True(t, empty)
}
