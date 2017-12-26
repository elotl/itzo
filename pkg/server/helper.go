package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
)

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

func unmountSpecial() {
	// Unmount in reverse order, since /dev/pts is inside /dev.
	for i := len(Mounts) - 1; i >= 0; i-- {
		m := Mounts[i]
		glog.Infof("Trying to Unmount() %s; this might fail", m.Target)
		if err := syscall.Unmount(m.Target, syscall.MNT_DETACH); err != nil {
			glog.Warningf("Unmount() %s: %v", m.Target, err)
		}
	}
}

func mountSpecial() error {
	for _, m := range Mounts {
		if err := os.MkdirAll(m.Target, 0700); err != nil {
			glog.Errorf("MkdirAll() %s: %v", m.Target, err)
			return err
		}
		glog.Infof("Mounting %s -> %s", m.Source, m.Target)
		if err := syscall.Mount(m.Source, m.Target, m.Fs, uintptr(m.Flags), m.Data); err != nil {
			glog.Errorf("Mount() %s -> %s: %v", m.Source, m.Target, err)
			unmountSpecial()
			return err
		}
	}
	return nil
}

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
	if rootfs != "" {
		if err := copyFile("/etc/resolv.conf", path.Join(rootfs, "/etc/resolv.conf")); err != nil {
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
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		glog.Errorf("Start() %v: %v", command, err)
		return err
	}
	glog.Infof("Unit %v under rootfs '%s' running as pid %d", command, rootfs, cmd.Process.Pid)

	err := cmd.Wait()
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
