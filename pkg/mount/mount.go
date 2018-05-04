package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/elotl/itzo/pkg/api"
	"github.com/golang/glog"
)

type (
	mountFunc   func(source string, target string, fstype string, flags uintptr, data string) error
	unmountFunc func(target string, flags int) error
	pivoterFunc func(rootfs, oldrootfs string) error
)

var (
	// Allow mocking out these syscalls in tests.
	mounter   mountFunc   = syscall.Mount
	unmounter unmountFunc = syscall.Unmount
	pivoter   pivoterFunc = syscall.PivotRoot
)

type Mounter interface {
	CreateMount(volume *api.Volume) error
	DeleteMount(volume *api.Volume) error
	AttachMount(unitname, src, dst string) error
	MountSpecial() error
	UnmountSpecial()
	BindMount(src, dst string) error
	Unmount(dir string) error
	PivotRoot(rootfs, oldrootfs string) error
}

type OSMounter struct {
	basedir string
}

type Mount struct {
	Source string
	Target string
	Fs     string
	Flags  int
	Data   string
}

var Mounts = []Mount{
	{
		Source: "proc",
		Target: "/proc",
		Fs:     "proc",
		Flags:  syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RELATIME,
	},
	{
		Source: "devtmpfs",
		Target: "/dev",
		Fs:     "devtmpfs",
		Flags:  syscall.MS_NOSUID | syscall.MS_RELATIME,
		Data:   "mode=755",
	},
	{
		Source: "devpts",
		Target: "/dev/pts",
		Fs:     "devpts",
		Flags:  syscall.MS_NOSUID | syscall.MS_NOEXEC | syscall.MS_RELATIME,
		// This data is from Alpine, might differ on other distributions.
		Data: "mode=620,ptmxmode=000",
	},
	{
		Source: "sysfs",
		Target: "/sys",
		Fs:     "sysfs",
		Flags:  syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RELATIME,
	},
}

func NewOSMounter(basedir string) Mounter {
	return &OSMounter{
		basedir: basedir,
	}
}

func (om *OSMounter) UnmountSpecial() {
	// Unmount in reverse order, since /dev/pts is inside /dev.
	for i := len(Mounts) - 1; i >= 0; i-- {
		m := Mounts[i]
		glog.Infof("Trying to Unmount() %s; this might fail", m.Target)
		if err := unmounter(m.Target, syscall.MNT_DETACH); err != nil {
			glog.Warningf("Unmount() %s: %v", m.Target, err)
		}
	}
}

func (om *OSMounter) MountSpecial() error {
	for _, m := range Mounts {
		if err := os.MkdirAll(m.Target, 0700); err != nil {
			glog.Errorf("MkdirAll() %s: %v", m.Target, err)
			return err
		}
		glog.Infof("Mounting %s -> %s", m.Source, m.Target)
		if err := mounter(m.Source, m.Target, m.Fs, uintptr(m.Flags), m.Data); err != nil {
			glog.Errorf("Mount() %s -> %s: %v", m.Source, m.Target, err)
			om.UnmountSpecial()
			return err
		}
	}
	return nil
}

func (om *OSMounter) BindMount(src, dst string) error {
	return mounter(src, dst, "", syscall.MS_BIND, "")
}

func (om *OSMounter) Unmount(dir string) error {
	return unmounter(dir, syscall.MNT_DETACH)
}

func (om *OSMounter) PivotRoot(rootfs, oldrootfs string) error {
	return pivoter(rootfs, oldrootfs)
}

func (om *OSMounter) CreateMount(volume *api.Volume) error {
	mountsdir := filepath.Join(om.basedir, "../mounts")
	err := os.MkdirAll(mountsdir, 0700)
	if err != nil {
		glog.Errorf("Error creating base mount directory %s: %v",
			mountsdir, err)
		return err
	}
	mdir := filepath.Join(mountsdir, volume.Name)
	_, err = os.Stat(mdir)
	if err != nil && !os.IsNotExist(err) {
		glog.Errorf("Error checking mount point %s: %v", mdir, err)
		return err
	}
	err = os.Mkdir(mdir, 0755)
	if err != nil {
		glog.Errorf("Error creating mount point %s: %v", mdir, err)
		return err
	}
	// For now, we only support EmptyDir. Later on we will need to check if
	// only one volume is in volspec.
	found := false
	if volume.EmptyDir != nil {
		found = true
		err = createEmptydir(mdir, volume.EmptyDir)
	}
	if !found {
		err = fmt.Errorf("No volume specified in %v", volume)
		glog.Errorf("%v", err)
	}
	return err
}

func (om *OSMounter) DeleteMount(volume *api.Volume) error {
	mdir := filepath.Join(om.basedir, "..", "mounts", volume.Name)
	_, err := os.Stat(mdir)
	if err != nil {
		glog.Errorf("Error accessing mount %s: %v", mdir, err)
	}
	// For now, we only support EmptyDir. Later on we will need to check if
	// only one volume is in volspec.
	found := false
	if volume.EmptyDir != nil {
		found = true
		switch volume.EmptyDir.Medium {
		case api.StorageMediumDefault:
			err = os.RemoveAll(mdir)
			if err != nil {
				glog.Errorf("Error removing emptyDir %s: %v", mdir, err)
			}
			return err
		case api.StorageMediumMemory:
			err = unmounter(mdir, syscall.MNT_DETACH)
			if err != nil {
				glog.Errorf("Error unmounting tmpfs %s: %v", mdir, err)
			}
			return err
		}
	}
	if !found {
		err = fmt.Errorf("No volume specified in %v", volume)
		glog.Errorf("%v", err)
	}
	return err
}

func (om *OSMounter) AttachMount(unit, src, dst string) error {
	source := filepath.Join(om.basedir, "../mounts", src)
	target := filepath.Join(om.basedir, unit, "ROOTFS", dst)
	err := os.MkdirAll(target, 0755)
	if err != nil {
		glog.Errorf("Error creating mount target directory %s: %v",
			target, err)
		return err
	}
	_, err = os.Stat(source)
	if err != nil {
		glog.Errorf("Error checking source mount point %s: %v", source, err)
		return err
	}
	// Bind mount source to target.
	err = mounter(source, target, "", uintptr(syscall.MS_BIND), "")
	if err != nil {
		glog.Errorf("Error mounting %s->%s: %v", source, target, err)
		return err
	}
	return nil
}

// Size is the amount of RAM in MB.
func createTmpfs(dir string, size int64) error {
	// Example: mount -t tmpfs -o size=512m tmpfs /mnt/mytmpfs
	sz := fmt.Sprintf("size=%dm", size)
	err := mounter("tmpfs", dir, "tmpfs", uintptr(0), sz)
	if err != nil {
		glog.Errorf("Failed to create tmpfs at %s: %v", dir, err)
		return err
	}
	return nil
}

func createEmptydir(dir string, emptyDir *api.EmptyDir) error {
	switch emptyDir.Medium {
	case api.StorageMediumDefault:
		glog.Infof("Using disk space for backing EmptyDir %s", dir)
		return nil
	case api.StorageMediumMemory:
		glog.Infof("Using tmpfs for backing EmptyDir %s", dir)
		return createTmpfs(dir, emptyDir.SizeLimit)
	}
	return fmt.Errorf("Unknown medium %s in createEmptydir()", emptyDir.Medium)
}