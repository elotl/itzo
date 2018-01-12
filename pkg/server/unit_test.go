package server

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

//func (u *Unit) GetRootfs() string {
//func (u *Unit) GetStatus() (UnitStatus, error) {
//func (u *Unit) SetStatus(status UnitStatus) error {

func TestNewUnit(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	defer os.RemoveAll(tmpdir)
	assert.Nil(t, err)
	u, err := NewUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer u.Close()
	uu, err := NewUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer uu.Close()
	assert.Equal(t, u.Name, uu.Name)
	assert.Equal(t, u.Directory, uu.Directory)
}

func TestNewUnitFromDir(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	defer os.RemoveAll(tmpdir)
	assert.Nil(t, err)
	u, err := NewUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer u.Close()
	uu, err := NewUnitFromDir(u.Directory)
	assert.Nil(t, err)
	defer uu.Close()
	assert.Equal(t, u.Name, uu.Name)
	assert.Equal(t, u.Directory, uu.Directory)
}

func TestGetRootfs(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	defer os.RemoveAll(tmpdir)
	assert.Nil(t, err)
	u, err := NewUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer u.Close()
	isEmpty, err := isEmptyDir(u.GetRootfs())
	assert.Nil(t, err)
	assert.True(t, isEmpty)
}

func TestStatus(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	defer os.RemoveAll(tmpdir)
	assert.Nil(t, err)
	u, err := NewUnit(tmpdir, "foobar")
	assert.Nil(t, err)
	defer u.Close()
	for _, s := range []UnitStatus{UnitStatusCreated, UnitStatusRunning, UnitStatusFailed, UnitStatusSucceeded} {
		err = u.SetStatus(s)
		assert.Nil(t, err)
		ss, err := u.GetStatus()
		assert.Nil(t, err)
		assert.Equal(t, s, ss)
	}
}
