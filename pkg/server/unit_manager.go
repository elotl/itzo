/*
Copyright 2020 Elotl Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/mount"
	"github.com/elotl/itzo/pkg/net"
	"github.com/elotl/itzo/pkg/util"
	"github.com/elotl/itzo/pkg/util/conmap"
	"github.com/golang/glog"
	quote "github.com/kballard/go-shellquote"
)

const (
	logBuffSize = 4096
)

func StartUnit(rootdir, podname, hostname, unitname, workingdir, netns string, command []string, policy api.RestartPolicy) error {
	unit, err := OpenUnit(rootdir, unitname)
	if err != nil {
		return err
	}
	mounter := mount.NewOSMounter(rootdir)
	nser := net.NewNoopNetNamespacer()
	if netns != "" && !api.IsHostNetwork(&unit.unitConfig.PodSecurityContext) {
		nser = net.NewOSNetNamespacer(netns)
	}
	glog.Infof("Starting %v for %s rootdir %s env %v workingdir %s policy %v",
		command, unitname, rootdir, os.Environ(), workingdir, policy)
	return nser.WithNetNamespace(func() error {
		return unit.Run(podname, hostname, command, workingdir, policy, mounter)
	})
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

	_, err := OpenUnit(um.rootDir, name)
	if err != nil {
		return fmt.Errorf("Error opening unit %s for termination: %s", name, err)
	}
	err = proc.Kill()
	if err != nil {
		// This happens if the process has already exited. Keep calm, log it
		// and carry on.
		glog.Warningf("Couldn't kill %s pid %d: %v (process terminated?)",
			name, proc.Pid, err)
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
func (um *UnitManager) StartUnit(podname, hostname, unitname, workingdir, netns string, command, args, appenv []string, policy api.RestartPolicy) error {
	glog.Infof("Starting unit %s", unitname)

	unit, err := OpenUnit(um.rootDir, unitname)
	if err != nil {
		return err
	}
	unitrootfs := unit.GetRootfs()

	if workingdir == "" {
		workingdir = unit.GetWorkingDir()
	}

	unitcmd := unit.CreateCommand(command, args)
	quotedcmd := quote.Join(unitcmd...)
	cmdline := []string{"--exec",
		quotedcmd,
		"--restartpolicy",
		string(policy),
		"--podname",
		podname,
		"--hostname",
		hostname,
		"--unit",
		unitname,
		"--rootdir",
		um.rootDir,
		"--workingdir",
		workingdir,
		"--netns",
		netns,
	}
	cmd := exec.Command("/proc/self/exe", cmdline...)

	env := unit.GetEnv() // Default environment from image config.
	for _, e := range appenv {
		// Add environment variables from the spec, overwriting default ones if
		// necessary.
		items := strings.SplitN(e, "=", 2)
		key, value := items[0], items[1]
		env = util.AddToEnvList(env, key, value, true)
	}
	cmd.Env = env

	glog.Infof("Unit %q workingdir %q command %v %v env %v policy %v",
		unitname, workingdir, command, args, env, policy)

	// Check if a chroot exists for the unit. If it does, a package has been
	// deployed there with a complete root filesystem, and we need to run our
	// command after chrooting into that rootfs.
	isUnitRootfsMissing, err := isEmptyDir(unitrootfs)
	if err != nil {
		glog.Errorf("Error checking if rootdir %s is an empty directory: %v",
			um.rootDir, err)
	}
	if !isUnitRootfsMissing {
		// If the parent mount of rootfs is shared, pivot_root will fail with
		// EINVAL. Adding CLONE_NEWNS to Unshareflags takes care of this, but
		// it also does it recursively (MS_REC), which might interfere if the
		// pod wants to share mounts under a rootfs subtree. We will make the
		// parent mount private right before calling pivot_root instead. Also
		// see https://go-review.googlesource.com/c/go/+/38471
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		}
	}

	um.CaptureLogs(unitname, unit)

	if err = cmd.Start(); err != nil {
		glog.Errorf("Failed to start %+v: %v", cmd, err)
		unit.LogPipe.Remove()
		return err
	}
	um.runningUnits.Set(unitname, cmd.Process)
	pid := cmd.Process.Pid
	go func() {
		err = cmd.Wait()
		if err == nil {
			glog.Infof("Unit %v (helper pid %d) exited", command, pid)
		} else {
			glog.Errorf("Unit %v (helper pid %d) exited with error %v", command, pid, err)
		}
		um.runningUnits.Delete(unitname)
		unit.LogPipe.Remove()
	}()
	return nil
}

func (um *UnitManager) CaptureLogs(name string, unit *Unit) {
	// XXX: Make number of log lines retained configurable.
	lp := unit.LogPipe
	um.logbuf.Set(name, logbuf.NewLogBuffer(logBuffSize))
	lp.StartReader(PIPE_UNIT_STDOUT, func(line string) {
		um.logbuf.Get(name).Write(logbuf.StdoutLogSource, line)
	})
	lp.StartReader(PIPE_UNIT_STDERR, func(line string) {
		um.logbuf.Get(name).Write(logbuf.StderrLogSource, line)
	})
	lp.StartReader(PIPE_HELPER_OUT, func(line string) {
		um.logbuf.Get(name).Write(logbuf.HelperLogSource, line)
	})
}
