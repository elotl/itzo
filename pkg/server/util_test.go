package server

import (
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyFile(t *testing.T) {
	buf := make([]byte, 9999)
	n, err := rand.Read(buf)
	assert.Nil(t, err)
	assert.Equal(t, len(buf), n)
	infile, err := ioutil.TempFile("", "itzo-test")
	assert.Nil(t, err)
	defer infile.Close()
	outfile, err := ioutil.TempFile("", "itzo-test")
	assert.Nil(t, err)
	defer outfile.Close()
	n, err = infile.Write(buf)
	assert.Nil(t, err)
	assert.Equal(t, len(buf), n)
	err = copyFile(infile.Name(), outfile.Name())
	assert.Nil(t, err)
	copiedBuf, err := ioutil.ReadFile(outfile.Name())
	assert.Nil(t, err)
	assert.Equal(t, buf, copiedBuf)
}

func TestIsEmptyDirMissing(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	os.RemoveAll(tmpdir)
	empty, err := isEmptyDir(tmpdir)
	assert.Nil(t, err)
	assert.True(t, empty)
}

func TestIsEmptyDirEmpty(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	defer os.RemoveAll(tmpdir)
	empty, err := isEmptyDir(tmpdir)
	assert.Nil(t, err)
	assert.True(t, empty)
}

func TestIsEmptyDirContainsFile(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	defer os.RemoveAll(tmpdir)
	empty, err := isEmptyDir(tmpdir)
	assert.Nil(t, err)
	assert.True(t, empty)
	f, err := os.Create(filepath.Join(tmpdir, "foo"))
	assert.Nil(t, err)
	defer f.Close()
	f.Write([]byte("bar"))
	empty, err = isEmptyDir(tmpdir)
	assert.Nil(t, err)
	assert.False(t, empty)
}

func TestIsEmptyDirContainsDir(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	defer os.RemoveAll(tmpdir)
	empty, err := isEmptyDir(tmpdir)
	assert.Nil(t, err)
	assert.True(t, empty)
	err = os.Mkdir(filepath.Join(tmpdir, "foo"), 0700)
	assert.Nil(t, err)
	empty, err = isEmptyDir(tmpdir)
	assert.Nil(t, err)
	assert.False(t, empty)
}

func TestEnsureFileExists(t *testing.T) {
	f, err := ioutil.TempFile("", "itzo-test")
	assert.Nil(t, err)
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	err = ensureFileExists(name)
	assert.Nil(t, err)
}

func TestEnsureFileExistsCreate(t *testing.T) {
	f, err := ioutil.TempFile("", "itzo-test")
	assert.Nil(t, err)
	name := f.Name()
	f.Close()
	os.Remove(name)
	_, err = os.Open(name)
	assert.True(t, os.IsNotExist(err))
	err = ensureFileExists(name)
	assert.Nil(t, err)
	_, err = os.Open(name)
	assert.False(t, os.IsNotExist(err))
}

func TestEnsureFileExistsFail(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	assert.Nil(t, err)
	os.RemoveAll(tmpdir)
	name := filepath.Join(tmpdir, "foobar")
	err = ensureFileExists(name)
	assert.NotNil(t, err)
	_, err = os.Open(name)
	assert.True(t, os.IsNotExist(err))
}
