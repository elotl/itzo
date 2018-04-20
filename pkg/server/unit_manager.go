package server

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/mount"
	"github.com/golang/glog"
)

func StartUnit(rootdir, name string, command []string, policy api.RestartPolicy) error {
	// todo: should this be rootdir or basedir?
	glog.Infof("Starting %v for %s in basedir %s", command, name, rootdir)
	unit, err := OpenUnit(rootdir, name)
	if err != nil {
		return err
	}
	// TODO: should this be rootdir or basedir?
	mounter := mount.NewOSMounter(rootdir)
	return unit.Run(command, os.Environ(), policy, mounter)
}

type UnitManager struct {
	rootDir      string
	runningUnits map[string]*os.Process
	logbuf       map[string]*LogBuffer
	logLock      sync.Mutex
}

func NewUnitManager(rootDir string) *UnitManager {
	return &UnitManager{
		rootDir:      rootDir,
		runningUnits: make(map[string]*os.Process),
		logbuf:       make(map[string]*LogBuffer),
		logLock:      sync.Mutex{},
	}
}

func (um *UnitManager) GetLogBuffer(unit string, n int) ([]LogEntry, error) {
	um.logLock.Lock()
	defer um.logLock.Unlock()
	if unit == "" {
		if len(um.logbuf) == 0 {
			return nil, fmt.Errorf("Unable to get logs, no units found")
		}
		if len(um.logbuf) == 1 {
			for _, lb := range um.logbuf {
				return lb.Read(n), nil
			}
		} else if len(um.runningUnits) == 1 {
			// we keep old logs around after a unit stops so
			// grab the logs from the only running unit if we can
			for name, _ := range um.runningUnits {
				lb, exists := um.logbuf[name]
				if exists {
					return lb.Read(n), nil
				}
			}
		}
		return nil, fmt.Errorf("Multiple unit logfiles found, please specify a unit name")
	}
	lb, exists := um.logbuf[unit]
	if !exists {
		return nil, fmt.Errorf("Could not find logs for unit named %s", unit)
	}
	return lb.Read(n), nil
}

// It's possible we need to set up some communication with the waiting
// process that it doesn't need to clean up everything.  Lets see how
// the logging works out...
func (um *UnitManager) StopUnit(name string) error {
	proc, exists := um.runningUnits[name]
	if !exists {
		return fmt.Errorf("Could not stop unit %s: Unit does not exist", name)
	}

	unit, err := OpenUnit(um.rootDir, name)
	if err != nil {
		return fmt.Errorf("Error opening unit %s for termination: %s", name, err)
	}
	unit.Destroy()
	err = proc.Kill()
	if err != nil {
		return fmt.Errorf("Error terminating %s: %v", unit, err)
	}
	delete(um.runningUnits, name)
	return nil
}

// This is a bit tricky in Go, since we are not supposed to use fork().
// Instead, call the daemon with command line flags indicating that it is only
// used as a helper to start a new unit in a new filesystem namespace.
func (um *UnitManager) StartUnit(name, command string, appenv []string, policy api.RestartPolicy) error {
	unit, err := OpenUnit(um.rootDir, name)
	if err != nil {
		return err
	}
	unitrootfs := unit.GetRootfs()

	cmdline := []string{
		"--exec",
		command,
		"--restartpolicy",
		string(policy),
		"--unit",
		name,
		"--rootdir",
		um.rootDir,
	}
	cmd := exec.Command("/proc/self/exe", cmdline...)
	cmd.Env = appenv

	// Check if a chroot exists for the unit. If it does, a package has been
	// deployed there with a complete root filesystem, and we need to run our
	// command after chrooting into that rootfs.
	isUnitRootfsMissing, err := isEmptyDir(unitrootfs)
	if err != nil {
		glog.Errorf("Error checking if rootdir %s is an empty directory: %v",
			um.rootDir, err)
	}
	if !isUnitRootfsMissing {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		}
	}

	lp := unit.LogPipe
	// XXX: Make number of log lines retained configurable.
	um.logLock.Lock()
	um.logbuf[name] = NewLogBuffer(1000)
	um.logLock.Unlock()
	lp.StartReader(PIPE_UNIT_STDOUT, func(line string) {
		um.logLock.Lock()
		um.logbuf[name].Write(fmt.Sprintf("[%s stdout]", name), line)
		um.logLock.Unlock()
	})
	lp.StartReader(PIPE_UNIT_STDERR, func(line string) {
		um.logLock.Lock()
		um.logbuf[name].Write(fmt.Sprintf("[%s stderr]", name), line)
		um.logLock.Unlock()
	})
	lp.StartReader(PIPE_HELPER_OUT, func(line string) {
		um.logLock.Lock()
		um.logbuf[name].Write(fmt.Sprintf("[%s helper]", name), line)
		um.logLock.Unlock()
	})

	if err = cmd.Start(); err != nil {
		lp.Remove()
		return err
	}
	um.runningUnits[name] = cmd.Process
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
	}()
	return nil
}
