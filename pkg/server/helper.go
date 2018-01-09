package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
)

const (
	ITZO_UNITDIR        = "ITZO_UNITDIR"
	ITZO_RESTART_POLICY = "ITZO_RESTART_POLICY"
	MAX_BACKOFF_TIME    = 5 * time.Minute
)

type RestartPolicy int

const (
	RESTART_POLICY_ALWAYS    RestartPolicy = iota
	RESTART_POLICY_NEVER     RestartPolicy = iota
	RESTART_POLICY_ONFAILURE RestartPolicy = iota
)

var logbuf = make(map[string]*LogBuffer)

func runUnit(command, env []string, unitout, uniterr *os.File, policy RestartPolicy) (err error) {
	backoff := 1 * time.Second
	for {
		start := time.Now()
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Env = env
		cmd.Stdout = unitout
		cmd.Stderr = uniterr

		err = cmd.Start()
		if err != nil {
			// Start() failed, it is either an error looking up the executable,
			// or a resource allocation problem.
			glog.Errorf("Start() %v: %v", command, err)
			return err
		}
		glog.Infof("Command %v running as pid %d", command, cmd.Process.Pid)

		err = cmd.Wait()
		d := time.Since(start)
		if err != nil {
			foundRc := false
			if exiterr, ok := err.(*exec.ExitError); ok {
				if ws, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					foundRc = true
					glog.Infof("Command %v pid %d exited with %d after %.2fs",
						command, cmd.Process.Pid, ws.ExitStatus(), d.Seconds())
				}
			}
			if !foundRc {
				glog.Infof("Command %v pid %d exited with %v after %.2fs",
					command, cmd.Process.Pid, err, d.Seconds())
			}
		} else {
			glog.Infof("Command %v pid %d exited with 0 after %.2fs",
				command, cmd.Process.Pid, d.Seconds())
		}

		switch policy {
		case RESTART_POLICY_NEVER:
			return err
		case RESTART_POLICY_ONFAILURE:
			if err == nil {
				return nil
			}
		case RESTART_POLICY_ALWAYS:
			// Fallthrough.
		}

		if err != nil {
			backoff *= 2
			if backoff > MAX_BACKOFF_TIME {
				backoff = MAX_BACKOFF_TIME
			}
		} else {
			// Reset backoff.
			backoff = 1 * time.Second
		}
		glog.Infof("Waiting for %v before starting %v again", backoff, command)
		time.Sleep(backoff)
	}
}

func GetRestartPolicy(env []string) RestartPolicy {
	policy := RESTART_POLICY_ALWAYS // Default restart policy.
	for _, s := range env {
		parts := strings.Split(s, "=")
		if len(parts) != 2 {
			glog.Fatalf("Invalid environment variable setting: %s", s)
		}
		if strings.ToUpper(parts[0]) != strings.ToUpper(ITZO_RESTART_POLICY) {
			continue
		}
		rp := parts[1]
		switch rp {
		case "RESTART_POLICY_ALWAYS":
			policy = RESTART_POLICY_ALWAYS
		case "RESTART_POLICY_NEVER":
			policy = RESTART_POLICY_NEVER
		case "RESTART_POLICY_ONFAILURE":
			policy = RESTART_POLICY_ONFAILURE
		default:
			glog.Warningf("Unknown restart policy %s, using default", rp)
		}
	}
	return policy
}

func SetRestartPolicy(env *[]string, policy RestartPolicy) {
	out := []string{}
	for _, s := range *env {
		// Remove existing policy.
		parts := strings.SplitN(s, "=", 2)
		if len(parts) < 2 {
			glog.Fatalf("Invalid environment variable setting: %s", s)
		}
		if strings.ToUpper(parts[0]) != strings.ToUpper(ITZO_RESTART_POLICY) {
			out = append(out, s)
		}
	}
	switch policy {
	case RESTART_POLICY_ALWAYS:
		out = append(out,
			fmt.Sprintf("%s=%s", ITZO_RESTART_POLICY, "RESTART_POLICY_ALWAYS"))
	case RESTART_POLICY_NEVER:
		out = append(out,
			fmt.Sprintf("%s=%s", ITZO_RESTART_POLICY, "RESTART_POLICY_NEVER"))
	case RESTART_POLICY_ONFAILURE:
		out = append(out,
			fmt.Sprintf("%s=%s", ITZO_RESTART_POLICY, "RESTART_POLICY_ONFAILURE"))
	}
	*env = out
}

// Helper function to start a unit in a chroot.
func StartUnit(rootfs string, command []string, policy RestartPolicy) error {
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

	err = runUnit(command, os.Environ(), unitout, uniterr, policy)

	if rootfs != "" {
		unmountSpecial()
	}

	return err
}

// This is a bit tricky in Go, since we are not supposed to use fork().
// Instead, call the daemon with command line flags indicating that it is only
// used as a helper to start a new unit in a new filesystem namespace.
func startUnitHelper(rootdir, unit string, args, appenv []string, policy RestartPolicy) (appid int, err error) {
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
	// XXX: Make number of log lines retained configurable.
	logbuf[unit] = NewLogBuffer(1000)
	lp.StartReader(PIPE_UNIT_STDOUT, func(line string) {
		logbuf[unit].Write(fmt.Sprintf("[%s stdout]", unit), line)
	})
	lp.StartReader(PIPE_UNIT_STDERR, func(line string) {
		logbuf[unit].Write(fmt.Sprintf("[%s stderr]", unit), line)
	})
	lp.StartReader(PIPE_HELPER_OUT, func(line string) {
		logbuf[unit].Write(fmt.Sprintf("[%s helper]", unit), line)
	})

	// Set restart policy.
	SetRestartPolicy(&appenv, policy)
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
