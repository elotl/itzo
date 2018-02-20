package server

import (
	"fmt"
	"os"
	"strings"
	"testing"

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
	err := mountSpecial()
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
	err := mountSpecial()
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
	unmountSpecial()
	assert.Equal(t, 4, len(unmounts))
}

func TestAttachMount(t *testing.T) {
	mountSrc := ""
	mountDst := ""
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountSrc = source
		mountDst = target
		return nil
	}
	defer os.RemoveAll("/tmp/itzo-test-mount")
	err := os.MkdirAll("/tmp/itzo-test-mount/units", 0755)
	assert.Nil(t, err)
	err = os.MkdirAll("/tmp/itzo-test-mount/mounts/mountSrc", 0755)
	assert.Nil(t, err)
	err = attachMount("/tmp/itzo-test-mount/units", "unit123", "mountSrc", "/mountDst")
	assert.Nil(t, err)
	assert.True(t, strings.Contains(mountSrc, "/mountSrc"))
	assert.True(t, strings.Contains(mountDst, "/unit123"))
	assert.True(t, strings.Contains(mountDst, "/mountDst"))
}

func TestAttachMountFail(t *testing.T) {
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		return fmt.Errorf("Testing attachMount() error")
	}
	defer os.RemoveAll("/tmp/itzo-test-mount")
	err := os.MkdirAll("/tmp/itzo-test-mount/units", 0755)
	assert.Nil(t, err)
	err = os.MkdirAll("/tmp/itzo-test-mount/mounts/mountSrc", 0755)
	assert.Nil(t, err)
	err = attachMount("/tmp/itzo-test-mount/units", "unit123", "mountSrc", "/mountDst")
	assert.NotNil(t, err)
}

func TestCreateMountEmptyDirDisk(t *testing.T) {
	mountCalled := false
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalled = true
		return nil
	}
	defer os.RemoveAll("/tmp/itzo-test-mount")
	err := os.MkdirAll("/tmp/itzo-test-mount/units", 0755)
	assert.Nil(t, err)
	err = os.MkdirAll("/tmp/itzo-test-mount/mounts", 0755)
	assert.Nil(t, err)
	err = createMount(
		"/tmp/itzo-test-mount/units",
		`{
            "name": "test-mount-name",
            "emptyDir": {}
        }`)
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
	defer os.RemoveAll("/tmp/itzo-test-mount")
	err := os.MkdirAll("/tmp/itzo-test-mount/units", 0755)
	assert.Nil(t, err)
	err = os.MkdirAll("/tmp/itzo-test-mount/mounts", 0755)
	assert.Nil(t, err)
	err = createMount(
		"/tmp/itzo-test-mount/units",
		`{
            "name": "test-mount-name",
            "emptyDir": {
                "medium": "Memory",
                "sizeLimit": 128
            }
        }`)
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
	defer os.RemoveAll("/tmp/itzo-test-mount")
	err := os.MkdirAll("/tmp/itzo-test-mount", 0755)
	assert.Nil(t, err)
	err = createTmpfs("/tmp/itzo-test-mount", 128)
	assert.Nil(t, err)
	assert.True(t, mountCalled)
	assert.Equal(t, "tmpfs", mountSrc)
	assert.Equal(t, "tmpfs", mountFstype)
	assert.True(t, strings.Contains(mountDst, "/itzo-test-mount"))
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
	defer os.RemoveAll("/tmp/itzo-test-mount")
	err := os.MkdirAll("/tmp/itzo-test-mount", 0755)
	assert.Nil(t, err)
	err = createTmpfs("/tmp/itzo-test-mount", 128)
	assert.NotNil(t, err)
	assert.True(t, mountCalled)
	assert.Equal(t, "tmpfs", mountSrc)
	assert.Equal(t, "tmpfs", mountFstype)
	assert.True(t, strings.Contains(mountDst, "/itzo-test-mount"))
	assert.True(t, strings.Contains(mountData, "128m"))
}

func TestCreateEmptydirDisk(t *testing.T) {
	mountCalled := false
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalled = true
		return nil
	}
	defer os.RemoveAll("/tmp/itzo-test-mount")
	err := os.MkdirAll("/tmp/itzo-test-mount", 0755)
	assert.Nil(t, err)
	err = createEmptydir("/tmp/itzo-test-mount", &EmptyDir{})
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
	defer os.RemoveAll("/tmp/itzo-test-mount")
	err := os.MkdirAll("/tmp/itzo-test-mount", 0755)
	assert.Nil(t, err)
	err = createEmptydir("/tmp/itzo-test-mount", &EmptyDir{
		Medium:    "Memory",
		SizeLimit: 128,
	})
	assert.Nil(t, err)
	assert.True(t, mountCalled)
	assert.Equal(t, "tmpfs", mountSrc)
	assert.Equal(t, "tmpfs", mountFstype)
	assert.True(t, strings.Contains(mountDst, "/itzo-test-mount"))
	assert.True(t, strings.Contains(mountData, "128m"))
}

func TestCreateEmptydirTmpfsFail(t *testing.T) {
	mountCalled := false
	mounter = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalled = true
		return fmt.Errorf("Testing createEmptydir() failure")
	}
	defer os.RemoveAll("/tmp/itzo-test-mount")
	err := os.MkdirAll("/tmp/itzo-test-mount", 0755)
	assert.Nil(t, err)
	err = createEmptydir("/tmp/itzo-test-mount", &EmptyDir{
		Medium:    "Memory",
		SizeLimit: 128,
	})
	assert.NotNil(t, err)
	assert.True(t, mountCalled)
}