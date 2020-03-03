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

func TestTailFile(t *testing.T) {
	f, err := ioutil.TempFile("", "itzo-test")
	assert.NoError(t, err)
	defer f.Close()
	data := `1
2
3
4
5
6
7
8
9
0
`
	_, err = f.Write([]byte(data))
	assert.NoError(t, err)
	// Get the last 3 lines
	vals, err := tailFile(f.Name(), 3, 999)
	assert.NoError(t, err)
	expected := "8\n9\n0\n"
	assert.Equal(t, expected, vals)
	// Last three lines are also the last 6 bytes, get those
	vals, err = tailFile(f.Name(), 0, 6)
	assert.NoError(t, err)
	assert.Equal(t, expected, vals)
	// make sure we can get the whole thing
	vals, err = tailFile(f.Name(), 0, int64(len(data)))
	assert.NoError(t, err)
	assert.Equal(t, data, vals)
}
