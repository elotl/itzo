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
	MountSpecial(unitname string) error
	UnmountSpecial(unitname string)
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

func (om *OSMounter) UnmountSpecial(unitname string) {
	rootfs := "/"
	if unitname != "" {
		rootfs = filepath.Join(om.basedir, unitname, "ROOTFS")
	}
	// Unmount in reverse order, since /dev/pts is inside /dev.
	for i := len(Mounts) - 1; i >= 0; i-- {
		m := Mounts[i]
		target := filepath.Join(rootfs, m.Target)
		glog.Infof("Trying to Unmount() %s; this might fail", target)
		if err := unmounter(target, syscall.MNT_DETACH); err != nil {
			glog.Warningf("Unmount() %s: %v", target, err)
		}
	}
}

func (om *OSMounter) MountSpecial(unitname string) error {
	rootfs := "/"
	if unitname != "" {
		rootfs = filepath.Join(om.basedir, unitname, "ROOTFS")
	}
	for _, m := range Mounts {
		target := filepath.Join(rootfs, m.Target)
		if err := os.MkdirAll(target, 0700); err != nil {
			glog.Errorf("MkdirAll() %s: %v", target, err)
			return err
		}
		glog.Infof("Mounting %s -> %s", m.Source, target)
		if err := mounter(m.Source, target, m.Fs, uintptr(m.Flags), m.Data); err != nil {
			glog.Errorf("Mount() %s -> %s: %v", m.Source, target, err)
			om.UnmountSpecial(unitname)
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
		glog.Errorf("error creating base mount directory %s: %v",
			mountsdir, err)
		return err
	}
	mountpath := filepath.Join(mountsdir, volume.Name)
	_, err = os.Stat(mountpath)
	if err != nil && !os.IsNotExist(err) {
		glog.Errorf("error checking mount point %s: %v", mountpath, err)
		return err
	}
	// For now, we only support EmptyDir and PackagePath volumes.
	found := false
	if volume.EmptyDir != nil {
		found = true
		err = createEmptydir(mountpath, volume.EmptyDir)
		if err != nil {
			glog.Errorf("error creating emptyDir %s: %v", mountpath, err)
			return err
		}
	}
	if volume.PackagePath != nil ||
		volume.Secret != nil ||
		volume.ConfigMap != nil {
		if found {
			err = fmt.Errorf("multiple volumes are specified in %v", volume)
			glog.Errorf("%v", err)
			return err
		}
		found = true
		var packagepath = filepath.Join(om.basedir, "..", "packages", volume.Name)
		if volume.PackagePath != nil {
			packagepath = filepath.Join(packagepath, volume.PackagePath.Path)
		}
		_, err = os.Stat(packagepath)
		if err != nil {
			glog.Errorf("error checking mount source %s: %v", packagepath, err)
			return err
		}
		err = os.Symlink(packagepath, mountpath)
		if err != nil {
			glog.Errorf("error creating link %s->%s: %v",
				packagepath, mountpath, err)
			return err
		}
	}
	if !found {
		err = fmt.Errorf("no volume specified in %v", volume)
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
	if volume.PackagePath != nil {
		if found {
			err = fmt.Errorf("Multiple volumes are specified in %v", volume)
			glog.Errorf("%v", err)
			return err
		}
		found = true
		err = os.RemoveAll(mountpath)
		if err != nil {
			glog.Errorf("Error removing PackagePath %s: %v", mountpath, err)
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

func ShareMount(target string, flags uintptr) error {
	glog.Infof("Setting sharing of mount at %s to %d", target, flags)
	return mounter("none", target, "", flags, "")
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
	err = mounter(source, target, "", uintptr(syscall.MS_BIND|syscall.MS_REC), "")
	if err != nil {
		glog.Errorf("Error mounting %s->%s: %v", source, target, err)
		return err
	}
	// Mark the mount as shared. This ensures we can share devices
	// mounted into emptyDirs between namespaces.  Shouldn't be too
	// strange for packagePaths unless people start mounting things
	// into their packages...
	err = ShareMount(target, uintptr(syscall.MS_SHARED|syscall.MS_REC))
	if err != nil {
		glog.Errorf("Error sharing mount %s: %v", target, err)
		return err
	}
	return nil
}

func (om *OSMounter) DetachMount(unit, dst string) error {
	base := filepath.Join(om.basedir, unit, "ROOTFS")
	target, err := resolveLinks(base, dst)
	if err != nil {
		glog.Errorf("Error resolving links in %s %s: %v", base, dst, err)
		return err
	}
	glog.Infof("Unmounting %s, actual path %s", dst, target)
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

	// rbind mount the emptydir onto itself so we can set mount
	// sharing parameters for anything mounted within the emptydir and
	// share mounted volumes through the emptydir.
	glog.Infof("Bind mounting Emptydir onto itself")
	err = mounter(dir, dir, "", uintptr(syscall.MS_BIND|syscall.MS_REC), "")
	if err != nil {
		glog.Errorf("Error bindmounting emptydir at %s: %v", dir, err)
		return err
	}
	err = ShareMount(dir, uintptr(syscall.MS_SHARED|syscall.MS_REC))
	if err != nil {
		glog.Errorf("Error sharing emptydir mount at %s: %v", dir, err)
		return err
	}
	if err != nil {
		glog.Errorf("Error making emptydir %s a shared mount point: %v", dir, err)
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
