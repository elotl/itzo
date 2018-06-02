package server

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/mount"
	"github.com/elotl/itzo/pkg/util/conmap"
	"github.com/golang/glog"
	quote "github.com/kballard/go-shellquote"
)

func StartUnit(rootdir, name string, command []string, policy api.RestartPolicy) error {
	// todo: should this be rootdir or basedir?
	glog.Infof("Starting %v for %s in basedir %s", command, name, rootdir)
	unit, err := OpenUnit(rootdir, name)
	if err != nil {
		return err
	}
	mounter := mount.NewOSMounter(rootdir)
	return unit.Run(command, os.Environ(), policy, mounter)
}

type UnitManager struct {
	rootDir      string
	runningUnits *conmap.StringOsProcess
	logbuf       *conmap.StringLogbufLogBuffer
}

func NewUnitManager(rootDir string) *UnitManager {
	return &UnitManager{
		rootDir:      rootDir,
		runningUnits: conmap.NewStringOsProcess(),
		logbuf:       conmap.NewStringLogbufLogBuffer(),
	}
}

func (um *UnitManager) GetLogBuffer(unit string) (*logbuf.LogBuffer, error) {
	lb, exists := um.logbuf.GetOK(unit)
	if !exists || lb == nil {
		return nil, fmt.Errorf("Could not find logs for unit named %s", unit)
	}
	return lb, nil
}

func (um *UnitManager) GetPid(unitName string) (int, bool) {
	proc, exists := um.runningUnits.GetOK(unitName)
	if !exists {
		return 0, false
	}
	return proc.Pid, true
}

func (um *UnitManager) ReadLogBuffer(unit string, n int) ([]logbuf.LogEntry, error) {
	if unit == "" {
		return nil, fmt.Errorf("Could not find unit")
	}
	lb, exists := um.logbuf.GetOK(unit)
	if !exists {
		return nil, fmt.Errorf("Could not find logs for unit named %s", unit)
	}
	return lb.Read(n), nil
}

func (um *UnitManager) UnitRunning(unit string) bool {
	_, exists := um.runningUnits.GetOK(unit)
	return exists
}

// It's possible we need to set up some communication with the waiting
// process that it doesn't need to clean up everything.  Lets see how
// the logging works out...
func (um *UnitManager) StopUnit(name string) error {
	proc, exists := um.runningUnits.GetOK(name)
	if !exists {
		return fmt.Errorf("Could not stop unit %s: Unit does not exist", name)
	}

	unit, err := OpenUnit(um.rootDir, name)
	if err != nil {
		return fmt.Errorf("Error opening unit %s for termination: %s", name, err)
	}
	err = proc.Kill()
	if err != nil {
		// This happens if the process has already exited. Keep calm, log it
		// and carry on.
		glog.Warningf("Couldn't kill %s pid %d: %v (process terminated?)",
			unit, proc.Pid, err)
	}
	um.runningUnits.Delete(name)
	return nil
}

// This removes the unit and its files/directories from the filesystem.
func (um *UnitManager) RemoveUnit(name string) error {
	unit, err := OpenUnit(um.rootDir, name)
	if err != nil {
		return fmt.Errorf("Error opening unit %s for removal: %v", name, err)
	}
	err = unit.Destroy()
	if err != nil {
		return fmt.Errorf("Error removing unit %s: %v", name, err)
	}
	return nil
}

// This is a bit tricky in Go, since we are not supposed to use fork().
// Instead, call the daemon with command line flags indicating that it is only
// used as a helper to start a new unit in a new filesystem namespace.
func (um *UnitManager) StartUnit(name string, command, args, appenv []string, policy api.RestartPolicy) error {
	unit, err := OpenUnit(um.rootDir, name)
	if err != nil {
		return err
	}
	unitrootfs := unit.GetRootfs()

	unitcmd := unit.CreateCommand(command, args)
	quotedcmd := quote.Join(unitcmd...)
	cmdline := []string{"--exec",
		quotedcmd,
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
	um.logbuf.Set(name, logbuf.NewLogBuffer(1000))
	lp.StartReader(PIPE_UNIT_STDOUT, func(line string) {
		um.logbuf.Get(name).Write(fmt.Sprintf("[%s stdout]", name), line)
	})
	lp.StartReader(PIPE_UNIT_STDERR, func(line string) {
		um.logbuf.Get(name).Write(fmt.Sprintf("[%s stderr]", name), line)
	})
	lp.StartReader(PIPE_HELPER_OUT, func(line string) {
		um.logbuf.Get(name).Write(fmt.Sprintf("[%s helper]", name), line)
	})

	if err = cmd.Start(); err != nil {
		lp.Remove()
		return err
	}
	um.runningUnits.Set(name, cmd.Process)
	pid := cmd.Process.Pid
	go func() {
		err = cmd.Wait()
		if err == nil {
			glog.Infof("Unit %v (helper pid %d) exited", command, pid)
		} else {
			glog.Errorf("Unit %v (helper pid %d) exited with error %v", command, pid, err)
		}
		lp.Remove()
		unit.Close()
	}()
	return nil
}
