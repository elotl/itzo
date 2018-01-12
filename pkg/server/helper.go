package server

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/golang/glog"
)

var logbuf = make(map[string]*LogBuffer)

// Helper function to start a unit in a chroot.
func StartUnit(unitdir string, command []string, policy RestartPolicy) error {
	glog.Infof("Starting new unit %v in %s", command, unitdir)
	unit, err := NewUnitFromDir(unitdir)
	if err != nil {
		return err
	}
	return unit.Run(command, os.Environ(), policy)
}

// This is a bit tricky in Go, since we are not supposed to use fork().
// Instead, call the daemon with command line flags indicating that it is only
// used as a helper to start a new unit in a new filesystem namespace.
func startUnitHelper(rootdir, name string, args, appenv []string, policy RestartPolicy) (appid int, err error) {
	unit, err := NewUnit(rootdir, name)
	if err != nil {
		return 0, err
	}
	unitdir := unit.Directory
	if err = os.MkdirAll(unitdir, 0700); err != nil {
		glog.Errorf("Error creating unit directory %s: %v", unitdir, err)
		return 0, err
	}
	unitrootfs := unit.GetRootfs()

	cmdline := []string{
		"--exec",
		strings.Join(args, " "),
		"--restartpolicy",
		RestartPolicyToString(policy),
		"--unitdir",
		unitdir,
	}
	cmd := exec.Command("/proc/self/exe", cmdline...)
	cmd.Env = appenv

	// Check if a chroot exists for the unit. If it does, a package has been
	// deployed there with a complete root filesystem, and we need to run our
	// command after chrooting into that rootfs.
	isUnitRootfsMissing, err := isEmptyDir(unitrootfs)
	if err != nil {
		glog.Errorf("Error checking if rootdir %s is an empty directory: %v",
			rootdir, err)
	}
	if !isUnitRootfsMissing {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		}
	}

	lp := unit.LogPipe
	// XXX: Make number of log lines retained configurable.
	logbuf[name] = NewLogBuffer(1000)
	lp.StartReader(PIPE_UNIT_STDOUT, func(line string) {
		logbuf[name].Write(fmt.Sprintf("[%s stdout]", name), line)
	})
	lp.StartReader(PIPE_UNIT_STDERR, func(line string) {
		logbuf[name].Write(fmt.Sprintf("[%s stderr]", name), line)
	})
	lp.StartReader(PIPE_HELPER_OUT, func(line string) {
		logbuf[name].Write(fmt.Sprintf("[%s helper]", name), line)
	})

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
		unit.Close()
		// XXX: use LogBuffer via Units, too.
	}()
	return pid, nil
}

func getLogBuffer(unit string, n int) []LogEntry {
	if unit == "" {
		// Logs from the first unit in the map, if there's any.
		for _, v := range logbuf {
			return v.Read(n)
		}
		// No units.
		return nil
	}
	lb := logbuf[unit]
	if lb != nil {
		return lb.Read(n)
	}
	return nil
}
