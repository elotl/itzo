package server

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/golang/glog"
)

// Helper function to start a unit in a chroot.
func StartUnit(rootfs string, command []string) error {
	glog.Infof("Starting new unit %v under rootfs '%s'", command, rootfs)
	if rootfs != "" {
		oldrootfs := fmt.Sprintf("%s/.oldrootfs", rootfs)
		if err := syscall.Mount(rootfs, rootfs, "", syscall.MS_BIND, ""); err != nil {
			glog.Errorf("Mount() %s: %v", rootfs, err)
			return err
		}
		if err := os.MkdirAll(oldrootfs, 0700); err != nil {
			glog.Errorf("MkdirAll() %s: %v", rootfs, err)
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
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		glog.Errorf("Start() %v: %v", command, err)
		return err
	}
	glog.Infof("Unit %v under rootfs '%s' running as pid %d", command, rootfs, cmd.Process.Pid)

	return cmd.Wait()
}
