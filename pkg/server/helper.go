package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
)

const (
	ITZO_UNITDIR = "ITZO_UNITDIR"
)

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

// Helper function to start a unit in a chroot.
func StartUnit(rootfs string, command []string) error {
	glog.Infof("Starting new unit %v under rootfs '%s'", command, rootfs)

	unitdir := os.Getenv(ITZO_UNITDIR)
	if unitdir == "" {
		return fmt.Errorf("Missing environment variable ITZO_UNITDIR")
	}

	// Open log pipes _before_ chrooting, since the named pipes are outside of
	// the rootfs.
	lp, err := NewLogPipe(unitdir)
	if err != nil {
		return err
	}
	helperout, err := lp.OpenWriter(PIPE_HELPER_OUT, true)
	if err != nil {
		lp.Remove()
		return err
	}
	defer helperout.Close()
	unitout, err := lp.OpenWriter(PIPE_UNIT_STDOUT, false)
	if err != nil {
		lp.Remove()
		return err
	}
	defer unitout.Close()
	uniterr, err := lp.OpenWriter(PIPE_UNIT_STDERR, false)
	if err != nil {
		lp.Remove()
		return err
	}
	defer uniterr.Close()

	if rootfs != "" {
		rootfsEtcDir := filepath.Join(rootfs, "/etc")
		if _, err := os.Stat(rootfsEtcDir); os.IsNotExist(err) {
			if err := os.Mkdir(rootfsEtcDir, 0755); err != nil {
				glog.Errorf("Could not make new rootfs/etc directory: %s", err)
				return err
			}
		}
		if err := copyFile("/etc/resolv.conf", filepath.Join(rootfs, "/etc/resolv.conf")); err != nil {
			glog.Errorf("copyFile() resolv.conf to %s: %v", rootfs, err)
			return err
		}
		oldrootfs := fmt.Sprintf("%s/.oldrootfs", rootfs)
		if err := syscall.Mount(rootfs, rootfs, "", syscall.MS_BIND, ""); err != nil {
			glog.Errorf("Mount() %s: %v", rootfs, err)
			return err
		}
		if err := os.MkdirAll(oldrootfs, 0700); err != nil {
			glog.Errorf("MkdirAll() %s: %v", oldrootfs, err)
			return err
		}
		if err := syscall.PivotRoot(rootfs, oldrootfs); err != nil {
			glog.Errorf("PivotRoot() %s %s: %v", rootfs, oldrootfs, err)
			return err
		}
		if err := os.Chdir("/"); err != nil {
			glog.Errorf("Chdir() /: %v", err)
			return err
		}
		if err := syscall.Unmount("/.oldrootfs", syscall.MNT_DETACH); err != nil {
			glog.Errorf("Unmount() %s: %v", oldrootfs, err)
			return err
		}
		os.Remove("/.oldrootfs")
		if err := mountSpecial(); err != nil {
			glog.Errorf("mountSpecial(): %v", rootfs, err)
			return err
		}
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = os.Environ() // Inherit all environment variables.
	cmd.Stdout = unitout
	cmd.Stderr = uniterr

	if err := cmd.Start(); err != nil {
		glog.Errorf("Start() %v: %v", command, err)
		return err
	}
	glog.Infof("Unit %v under rootfs '%s' running as pid %d", command, rootfs, cmd.Process.Pid)

	err = cmd.Wait()
	if rootfs != "" {
		unmountSpecial()
	}
	return err
}

func resizeVolume() error {
	mounts, err := os.Open("/proc/mounts")
	if err != nil {
		glog.Errorf("opening /proc/mounts: %v", err)
		return err
	}
	defer mounts.Close()
	rootdev := ""
	scanner := bufio.NewScanner(mounts)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) < 2 || parts[1] != "/" {
			continue
		} else {
			rootdev = parts[0]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		glog.Errorf("reading /proc/mounts: %v", err)
		return err
	}
	if rootdev == "" {
		err = fmt.Errorf("can't find device the root filesystem is mounted on")
		glog.Error(err)
		return err
	}
	for count := 0; count < 10; count++ {
		// It might take a bit of time for Xen and/or the kernel to detect
		// capacity changes on block devices. The output of resize2fs will
		// contain if it did not need to do anything ("Nothing to do!") vs when
		// it resized the device ("resizing required").
		cmd := exec.Command("resize2fs", rootdev)
		var outbuf bytes.Buffer
		var errbuf bytes.Buffer
		cmd.Stdout = io.MultiWriter(os.Stdout, &outbuf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &errbuf)
		glog.Infof("trying to resize %s", rootdev)
		if err := cmd.Start(); err != nil {
			glog.Errorf("resize2fs %s: %v", rootdev, err)
			return err
		}
		if err := cmd.Wait(); err != nil {
			glog.Errorf("resize2fs %s: %v", rootdev, err)
			return err
		}
		if strings.Contains(outbuf.String(), "resizing required") ||
			strings.Contains(errbuf.String(), "resizing required") {
			glog.Infof("%s has been successfully resized", rootdev)
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	glog.Errorf("resizing %s failed", rootdev)
	return fmt.Errorf("no resizing performed; does %s have new capacity?",
		rootdev)
}

func getUnitDir(rootdir, unit string) string {
	return filepath.Join(rootdir, unit)
}

func getUnitRootfs(rootdir, unit string) string {
	return filepath.Join(getUnitDir(rootdir, unit), "ROOTFS")
}

func isEmptyDir(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil && !os.IsExist(err) {
		return true, nil
	} else if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

// This is a bit tricky in Go, since we are not supposed to use fork().
// Instead, call the daemon with command line flags indicating that it is only
// used as a helper to start a new unit in a new filesystem namespace.
func startUnitHelper(rootdir, unit string, args, appenv []string) (appid int, err error) {
	unitdir := getUnitDir(rootdir, unit)
	if err = os.MkdirAll(unitdir, 0700); err != nil {
		glog.Errorf("Error creating unit directory %s: %v", unitdir, err)
		return 0, err
	}
	unitrootfs := getUnitRootfs(rootdir, unit)
	// Check if a chroot exists for the unit. If it does, a package has been
	// deployed there with a complete root filesystem, and we need to run our
	// command after chrooting into that rootfs.
	isUnitRootfsMissing, err := isEmptyDir(unitrootfs)
	if err != nil {
		glog.Errorf("Error checking if rootdir %s is an empty directory: %v",
			rootdir, err)
	}
	cmdline := []string{
		"--exec",
		strings.Join(args, " "),
	}
	if !isUnitRootfsMissing {
		// The rootfs of the unit is something like
		// "/tmp/milpa/units/foobar/ROOTFS". It is a complete root filesystem
		// we are supposed to chroot into.
		cmdline = append(cmdline, "--rootfs", unitrootfs)
	}
	cmd := exec.Command("/proc/self/exe", cmdline...)
	if !isUnitRootfsMissing {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		}
	}
	lp, err := NewLogPipe(unitdir)
	if err != nil {
		glog.Errorf("Error creating log pipes for %s: %v", unit, err)
		return 0, err
	}
	lp.StartAllReaders(func(line string) {
		prefix := fmt.Sprintf("[%s]", unit)
		glog.Infof("%s %s", prefix, line)
	})

	// Provide the location of the unit directory via an environment variable.
	cmd.Env = append(appenv, fmt.Sprintf("%s=%s", ITZO_UNITDIR, unitdir))
	if err = cmd.Start(); err != nil {
		lp.Remove()
		return 0, err
	}
	pid := cmd.Process.Pid
	go func() {
		err = cmd.Wait()
		if err == nil {
			glog.Infof("Unit %v (helper pid %d) exited", args, pid)
		} else {
			glog.Errorf("Unit %v (helper pid %d) exited with error %v", args, pid, err)
		}
		lp.Remove()
	}()
	return pid, nil
}
