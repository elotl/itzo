package server

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/elotl/itzo/pkg/mount"
	"github.com/golang/glog"
)

var (
	logbuf  = make(map[string]*LogBuffer)
	logLock = sync.Mutex{}
)

// Helper function to start a unit in a chroot.
func StartUnit(rootdir, name string, command []string, policy RestartPolicy) error {
	glog.Infof("Starting %v for %s in basedir %s", command, name, rootdir)
	unit, err := OpenUnit(rootdir, name)
	if err != nil {
		return err
	}
	mounter := mount.NewOSMounter(rootdir)
	return unit.Run(command, os.Environ(), policy, &mounter)
}

// It's possible we need to set up some communication with the waiting
// process that it doesn't need to clean up everything.  Lets see how
// the logging works out...
func stopUnitHelper(rootdir, name string, proc *os.Process) {
	unit, err := OpenUnit(rootdir, name)
	if err != nil {
		glog.Errorf("Error opening unit %s for termination: %s", name, err)
	}
	unit.Remove()
	err = proc.Kill()
	if err != nil {
		glog.Errorln("Error terminating", unit, err)
	}
}

// This is a bit tricky in Go, since we are not supposed to use fork().
// Instead, call the daemon with command line flags indicating that it is only
// used as a helper to start a new unit in a new filesystem namespace.
func startUnitHelper(rootdir, name, command string, appenv []string, policy RestartPolicy) (*os.Process, error) {
	unit, err := OpenUnit(rootdir, name)
	if err != nil {
		return nil, err
	}
	unitrootfs := unit.GetRootfs()

	cmdline := []string{
		"--exec",
		command,
		"--restartpolicy",
		RestartPolicyToString(policy),
		"--unit",
		name,
		"--rootdir",
		rootdir,
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
	logLock.Lock()
	logbuf[name] = NewLogBuffer(1000)
	logLock.Unlock()
	lp.StartReader(PIPE_UNIT_STDOUT, func(line string) {
		logLock.Lock()
		logbuf[name].Write(fmt.Sprintf("[%s stdout]", name), line)
		logLock.Unlock()
	})
	lp.StartReader(PIPE_UNIT_STDERR, func(line string) {
		logLock.Lock()
		logbuf[name].Write(fmt.Sprintf("[%s stderr]", name), line)
		logLock.Unlock()
	})
	lp.StartReader(PIPE_HELPER_OUT, func(line string) {
		logLock.Lock()
		logbuf[name].Write(fmt.Sprintf("[%s helper]", name), line)
		logLock.Unlock()
	})

	if err = cmd.Start(); err != nil {
		lp.Remove()
		return nil, err
	}
	pid := cmd.Process.Pid
	go func() {
		err = cmd.Wait()
		if err == nil {
			glog.Infof("Unit %s (helper pid %d) exited", command, pid)
		} else {
			glog.Errorf("Unit %s (helper pid %d) exited with error %v", command, pid, err)
		}
		lp.Remove()
		unit.Close()
		// XXX: use LogBuffer via Units, too.
	}()
	return cmd.Process, nil
}

func getLogBuffer(unit string, n int) []LogEntry {
	logLock.Lock()
	defer logLock.Unlock()
	if unit == "" && len(logbuf) == 1 {
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
