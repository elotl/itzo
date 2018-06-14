package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	DetachMount(unitname, dst string) error
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
	return mounter(src, dst, "", uintptr(syscall.MS_BIND|syscall.MS_REC), "")
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
	mountpath := filepath.Join(mountsdir, volume.Name)
	_, err = os.Stat(mountpath)
	if err != nil && !os.IsNotExist(err) {
		glog.Errorf("Error checking mount point %s: %v", mountpath, err)
		return err
	}
	// For now, we only support EmptyDir and HostPath volumes.
	found := false
	if volume.EmptyDir != nil {
		found = true
		err = createEmptydir(mountpath, volume.EmptyDir)
		if err != nil {
			glog.Errorf("Error creating emptyDir %s: %v", mountpath, err)
			return err
		}
	}
	if volume.HostPath != nil {
		if found {
			err = fmt.Errorf("Multiple volumes are specified in %v", volume)
			glog.Errorf("%v", err)
			return err
		}
		found = true
		packagepath := filepath.Join(
			om.basedir, "..", "packages", volume.Name, volume.HostPath.Path)
		_, err = os.Stat(packagepath)
		if err != nil {
			glog.Errorf("Error checking mount source %s: %v", packagepath, err)
			return err
		}
		err = os.Symlink(packagepath, mountpath)
		if err != nil {
			glog.Errorf("Error creating link %s->%s: %v",
				packagepath, mountpath, err)
			return err
		}
	}
	if !found {
		err = fmt.Errorf("No volume specified in %v", volume)
		glog.Errorf("%v", err)
		return err
	}
	return nil
}

func (om *OSMounter) DeleteMount(volume *api.Volume) error {
	mountpath := filepath.Join(om.basedir, "..", "mounts", volume.Name)
	_, err := os.Stat(mountpath)
	if err != nil {
		glog.Errorf("Error accessing mount %s: %v", mountpath, err)
	}
	// For now, we only support EmptyDir. Later on we will need to check if
	// only one volume is in volspec.
	found := false
	if volume.EmptyDir != nil {
		found = true
		switch volume.EmptyDir.Medium {
		case api.StorageMediumDefault:
			err = os.RemoveAll(mountpath)
			if err != nil {
				glog.Errorf("Error removing emptyDir %s: %v", mountpath, err)
				return err
			}
		case api.StorageMediumMemory:
			err = unmounter(mountpath, syscall.MNT_DETACH)
			if err != nil {
				glog.Errorf("Error unmounting tmpfs %s: %v", mountpath, err)
				return err
			}
		}
	}
	if volume.HostPath != nil {
		if found {
			err = fmt.Errorf("Multiple volumes are specified in %v", volume)
			glog.Errorf("%v", err)
			return err
		}
		found = true
		err = os.RemoveAll(mountpath)
		if err != nil {
			glog.Errorf("Error removing hostPath %s: %v", mountpath, err)
			return err
		}
	}
	if !found {
		err = fmt.Errorf("No volume specified in %v", volume)
		glog.Errorf("%v", err)
		return err
	}
	return nil
}

func resolveLinks(base, target string) (string, error) {
	targetList := strings.Split(target, string(filepath.Separator))
	var path string
	for i, _ := range targetList {
		path = filepath.Join(base, filepath.Join(targetList[:i+1]...))
		glog.Infof("Checking %s", path)
		fi, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				glog.Infof("%s does not exist", path)
				return filepath.Join(base, target), nil
			}
			glog.Errorf("Lstat() error on %s: %v", path, err)
			return "", err
		}
		if (fi.Mode() & os.ModeSymlink) != os.ModeSymlink {
			continue
		}
		dst, err := os.Readlink(path)
		if err != nil {
			glog.Errorf("Readlink() error on %s: %v", path, err)
			return "", err
		}
		if len(dst) > 0 && dst[0] == filepath.Separator {
			// Absolute link. It should stay inside the chroot, so prepend
			// base, and then check the rest of the path.
			base = filepath.Join(base, dst)
			glog.Infof("%s is absolute link, new base: %s", dst, base)
			return resolveLinks(base, filepath.Join(targetList[i+1:]...))
		}
	}
	return path, nil
}

func (om *OSMounter) AttachMount(unit, src, dst string) error {
	glog.Infof("Mounting %s->%s", src, dst)
	// Directory for mount source.
	source := filepath.Join(om.basedir, "../mounts", src)
	// Check symlinks in dst.
	base := filepath.Join(om.basedir, unit, "ROOTFS")
	target, err := resolveLinks(base, dst)
	if err != nil {
		glog.Errorf("Error resolving links in %s %s: %v", base, dst, err)
		return err
	}
	glog.Infof("Mounting %s->%s, actual path %s->%s", src, dst, source, target)
	// Create directory for target if necessary.
	fi, err := os.Stat(source)
	if err != nil {
		glog.Errorf("Error checking mount source %s: %v", source, err)
		return err
	}
	dir := filepath.Clean(target)
	if !fi.IsDir() {
		dir = filepath.Clean(filepath.Join(target, ".."))
		glog.Infof("Mount source %s is a file, creating dir at %s", source, dir)
	} else {
		glog.Infof("Mount source %s is a directory, creating dir at %s", source, dir)
	}
	if !filepath.HasPrefix(dir, base) {
		err = fmt.Errorf("Invalid mount target %s (%s is not in %s)",
			dst, dir, base)
		glog.Errorf("%v", err)
		return err
	}
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		glog.Errorf("Error creating mount target directory %s: %v", dir, err)
		return err
	}
	if dir != target {
		// Make sure target file exists, otherwise the bind mount will fail.
		f, err := os.Create(target)
		if err != nil {
			glog.Errorf("Error creating mount target file %s: %v", target, err)
			return err
		}
		f.Close()
	}
	// Bind mount source to target.
	err = mounter(source, target, "", uintptr(syscall.MS_BIND), "")
	if err != nil {
		glog.Errorf("Error mounting %s->%s: %v", source, target, err)
		return err
	}
	return nil
}

func (om *OSMounter) DetachMount(unit, dst string) error {
	target := filepath.Join(om.basedir, unit, "ROOTFS", dst)
	if err := unmounter(target, syscall.MNT_DETACH); err != nil {
		glog.Errorf("Error unmounting %s: %v", target, err)
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
	err := os.Mkdir(dir, 0755)
	if err != nil && !os.IsExist(err) {
		glog.Errorf("Error creating emptyDir mount point %s: %v", dir, err)
		return err
	}
	switch emptyDir.Medium {
	case api.StorageMediumDefault:
		glog.Infof("Using disk space for backing EmptyDir %s", dir)
		return nil
	case api.StorageMediumMemory:
		glog.Infof("Using tmpfs for backing EmptyDir %s", dir)
		return createTmpfs(dir, emptyDir.SizeLimit)
	}
	err = fmt.Errorf("Unknown medium %s in createEmptydir()", emptyDir.Medium)
	glog.Errorf("%v", err)
	return err
}
