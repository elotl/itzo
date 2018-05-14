package mount

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/elotl/itzo/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestMountSpecial(t *testing.T) {
	type mount struct {
		src    string
		dst    string
		fstype string
	}
	mounts := make([]mount, 0)
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mounts = append(mounts, mount{
			src:    source,
			dst:    target,
			fstype: fstype,
		})
		return nil
	}
	m := NewOSMounter("")
	err := m.MountSpecial()
	assert.Nil(t, err)
	assert.Equal(t, 4, len(mounts))
}

func TestMountSpecialFail(t *testing.T) {
	failedTarget := ""
	unmountTarget := ""
	unmountCalled := false
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		failedTarget = target
		return fmt.Errorf("Testing mount error")
	}
	unmounter = func(target string, flags int) error {
		unmountTarget = target
		unmountCalled = true
		return nil
	}
	m := NewOSMounter("")
	err := m.MountSpecial()
	assert.NotNil(t, err)
	assert.True(t, unmountCalled)
	assert.Equal(t, failedTarget, unmountTarget)
}

func TestUnmountSpecial(t *testing.T) {
	unmounts := make([]string, 0)
	unmounter = func(target string, flags int) error {
		unmounts = append(unmounts, target)
		return nil
	}
	m := NewOSMounter("")
	m.UnmountSpecial()
	assert.Equal(t, 4, len(unmounts))
}

func createTmpDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "itzo-tmpdir-")
	assert.NoError(t, err)
	return dir
}

func TestAttachMount(t *testing.T) {
	mountSrc := ""
	mountDst := ""
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountSrc = source
		mountDst = target
		return nil
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := os.MkdirAll(tmpdir+"/mounts/mountSrc", 0755)
	assert.Nil(t, err)
	m := NewOSMounter(tmpdir + "/units")
	err = m.AttachMount("unit123", "mountSrc", "/mountDst")
	assert.Nil(t, err)
	assert.True(t, strings.Contains(mountSrc, "/mountSrc"))
	assert.True(t, strings.Contains(mountDst, "/unit123"))
	assert.True(t, strings.Contains(mountDst, "/mountDst"))
}

func TestAttachMountFail(t *testing.T) {
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		return fmt.Errorf("Testing attachMount() error")
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := os.MkdirAll(tmpdir+"/units", 0755)
	assert.Nil(t, err)
	err = os.MkdirAll(tmpdir+"/mounts/mountSrc", 0755)
	assert.Nil(t, err)
	m := NewOSMounter(tmpdir + "/units")
	err = m.AttachMount("unit123", "mountSrc", "/mountDst")
	assert.NotNil(t, err)
}

func TestDetachMount(t *testing.T) {
	mountDst := ""
	unmounter = func(target string, flags int) error {
		mountDst = target
		return nil
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := os.MkdirAll(tmpdir+"/mounts/mountSrc", 0755)
	assert.NoError(t, err)
	m := NewOSMounter(tmpdir + "/units")
	err = m.DetachMount("unit123", "/mountDst")
	assert.NoError(t, err)
	assert.True(t, strings.Contains(mountDst, "/unit123"))
	assert.True(t, strings.Contains(mountDst, "/mountDst"))
}

func TestDetachMountFail(t *testing.T) {
	unmounter = func(target string, flags int) error {
		return fmt.Errorf("Testing DetachMount() error")
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := os.MkdirAll(tmpdir+"/units", 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(tmpdir+"/mounts/mountSrc", 0755)
	assert.NoError(t, err)
	m := NewOSMounter(tmpdir + "/units")
	err = m.DetachMount("unit123", "/mountDst")
	assert.Error(t, err)
}

func TestCreateMountEmptyDirDisk(t *testing.T) {
	mountCalled := false
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalled = true
		return nil
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := os.MkdirAll(tmpdir+"/units", 0755)
	assert.Nil(t, err)
	err = os.MkdirAll(tmpdir+"/mounts", 0755)
	assert.Nil(t, err)
	vol := api.Volume{
		Name: "test-mount-name",
		VolumeSource: api.VolumeSource{
			EmptyDir: &api.EmptyDir{},
		},
	}
	m := NewOSMounter(tmpdir + "/units")
	err = m.CreateMount(&vol)
	assert.Nil(t, err)
	assert.False(t, mountCalled)
}

func TestCreateMountEmptyDirTmpfs(t *testing.T) {
	mountCalled := false
	mountSrc := ""
	mountDst := ""
	mountFstype := ""
	mountData := ""
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalled = true
		mountSrc = source
		mountDst = target
		mountFstype = fstype
		mountData = data
		return nil
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := os.MkdirAll(tmpdir+"/units", 0755)
	assert.Nil(t, err)
	err = os.MkdirAll(tmpdir+"/mounts", 0755)
	assert.Nil(t, err)
	vol := api.Volume{
		Name: "test-mount-name",
		VolumeSource: api.VolumeSource{
			EmptyDir: &api.EmptyDir{
				Medium:    api.StorageMediumMemory,
				SizeLimit: 128,
			},
		},
	}
	m := NewOSMounter(tmpdir + "/units")
	err = m.CreateMount(&vol)
	assert.Nil(t, err)
	assert.True(t, mountCalled)
	assert.Equal(t, "tmpfs", mountSrc)
	assert.Equal(t, "tmpfs", mountFstype)
	assert.True(t, strings.Contains(mountDst, "/test-mount-name"))
	assert.True(t, strings.Contains(mountData, "128m"))
}

func TestCreateTmpfs(t *testing.T) {
	mountCalled := false
	mountSrc := ""
	mountDst := ""
	mountFstype := ""
	mountData := ""
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalled = true
		mountSrc = source
		mountDst = target
		mountFstype = fstype
		mountData = data
		return nil
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := createTmpfs(tmpdir, 128)
	assert.Nil(t, err)
	assert.True(t, mountCalled)
	assert.Equal(t, "tmpfs", mountSrc)
	assert.Equal(t, "tmpfs", mountFstype)
	assert.True(t, strings.Contains(mountDst, tmpdir))
	assert.True(t, strings.Contains(mountData, "128m"))
}

func TestCreateTmpfsFail(t *testing.T) {
	mountCalled := false
	mountSrc := ""
	mountDst := ""
	mountFstype := ""
	mountData := ""
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalled = true
		mountSrc = source
		mountDst = target
		mountFstype = fstype
		mountData = data
		return fmt.Errorf("Testing createTmpfs() failure")
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := createTmpfs(tmpdir, 128)
	assert.NotNil(t, err)
	assert.True(t, mountCalled)
	assert.Equal(t, "tmpfs", mountSrc)
	assert.Equal(t, "tmpfs", mountFstype)
	assert.True(t, strings.Contains(mountDst, tmpdir))
	assert.True(t, strings.Contains(mountData, "128m"))
}

func TestCreateEmptydirDisk(t *testing.T) {
	mountCalled := false
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalled = true
		return nil
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := createEmptydir(tmpdir, &api.EmptyDir{})
	fmt.Printf("createEmptydir(): %v\n", err)
	assert.Nil(t, err)
	assert.False(t, mountCalled)
}

func TestCreateEmptydirTmpfs(t *testing.T) {
	mountCalled := false
	mountSrc := ""
	mountDst := ""
	mountFstype := ""
	mountData := ""
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalled = true
		mountSrc = source
		mountDst = target
		mountFstype = fstype
		mountData = data
		return nil
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := createEmptydir(tmpdir, &api.EmptyDir{
		Medium:    "Memory",
		SizeLimit: 128,
	})
	assert.Nil(t, err)
	assert.True(t, mountCalled)
	assert.Equal(t, "tmpfs", mountSrc)
	assert.Equal(t, "tmpfs", mountFstype)
	assert.True(t, strings.Contains(mountDst, tmpdir))
	assert.True(t, strings.Contains(mountData, "128m"))
}

func TestCreateEmptydirTmpfsFail(t *testing.T) {
	mountCalled := false
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalled = true
		return fmt.Errorf("Testing createEmptydir() failure")
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := createEmptydir(tmpdir, &api.EmptyDir{
		Medium:    "Memory",
		SizeLimit: 128,
	})
	assert.NotNil(t, err)
	assert.True(t, mountCalled)
}

func TestDeleteDiskBackedEmptyDirMount(t *testing.T) {
	unmountCalled := false
	unmounter = func(target string, flags int) error {
		unmountCalled = true
		return nil
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := os.MkdirAll(tmpdir+"/mounts", 0755)
	assert.Nil(t, err)
	vol := api.Volume{
		Name: "test-mount-name",
		VolumeSource: api.VolumeSource{
			EmptyDir: &api.EmptyDir{},
		},
	}
	m := NewOSMounter(tmpdir + "/units")
	err = m.DeleteMount(&vol)
	assert.Nil(t, err)
	assert.False(t, unmountCalled)
}

func TestDeleteMemoryBackedEmptyDirMount(t *testing.T) {
	unmountCalled := false
	unmounter = func(target string, flags int) error {
		unmountCalled = true
		return nil
	}
	tmpdir := createTmpDir(t)
	defer os.RemoveAll(tmpdir)
	err := os.MkdirAll(tmpdir+"/mounts", 0755)
	assert.Nil(t, err)
	vol := api.Volume{
		Name: "test-mount-name",
		VolumeSource: api.VolumeSource{
			EmptyDir: &api.EmptyDir{
				Medium:    "Memory",
				SizeLimit: 128,
			},
		},
	}
	m := NewOSMounter(tmpdir + "/units")
	err = m.DeleteMount(&vol)
	assert.Nil(t, err)
	assert.True(t, unmountCalled)
}
