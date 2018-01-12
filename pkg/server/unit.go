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
	MAX_BACKOFF_TIME = 5 * time.Minute
)

type Unit struct {
	*LogPipe
	Directory  string
	Name       string
	statusFile *os.File
}

type UnitStatus string

const (
	UnitStatusUnknown   = ""
	UnitStatusCreated   = "created"
	UnitStatusRunning   = "running"
	UnitStatusFailed    = "failed"
	UnitStatusSucceeded = "succeeded"
)

type RestartPolicy int

const (
	RESTART_POLICY_ALWAYS    RestartPolicy = iota
	RESTART_POLICY_NEVER     RestartPolicy = iota
	RESTART_POLICY_ONFAILURE RestartPolicy = iota
)

func NewUnit(rootdir, name string) (*Unit, error) {
	glog.Infof("Creating new unit '%s' in %s\n", name, rootdir)
	directory := filepath.Join(rootdir, name)
	// Make sure unit directory exists.
	if err := os.MkdirAll(directory, 0700); err != nil {
		glog.Errorf("Error reating unit '%s': %v\n", name, err)
		return nil, err
	}
	spath := filepath.Join(directory, "status")
	f, err := os.OpenFile(spath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		glog.Errorf("Error opening statusfile for unit '%s': %v\n", name, err)
		return nil, err
	}
	lp, err := NewLogPipe(directory)
	if err != nil {
		f.Close()
		glog.Errorf("Error creating logpipes for unit '%s': %v\n", name, err)
		return nil, err
	}
	u := Unit{
		LogPipe:    lp,
		Directory:  directory,
		Name:       name,
		statusFile: f,
	}
	u.SetStatus(UnitStatusCreated)
	return &u, nil
}

func NewUnitFromDir(unitdir string) (*Unit, error) {
	elements := strings.Split(unitdir, string(filepath.Separator))
	if elements[len(elements)-1] == "" {
		elements = elements[:len(elements)-1]
	}
	if len(elements) <= 1 {
		return nil, fmt.Errorf("Invalid unitdir %s", unitdir)
	}
	rootdir := strings.Join(elements[:len(elements)-1], string(filepath.Separator))
	name := elements[len(elements)-1]
	return NewUnit(rootdir, name)
}

func (u *Unit) Close() {
	if u.statusFile != nil {
		name := u.statusFile.Name()
		u.statusFile.Close()
		u.statusFile = nil
		os.Remove(name)
	}
}

func (u *Unit) GetRootfs() string {
	return filepath.Join(u.Directory, "ROOTFS")
}

// Note: even though multiple goroutines might call GetStatus(), they are
// expected to create their own instances of Unit, thus statusFile is _not_
// shared. Only the helper process calls SetStatus(), so writes to the
// statusfile don't need to be locked either.
func (u *Unit) GetStatus() (UnitStatus, error) {
	_, err := u.statusFile.Seek(0, 0)
	if err != nil {
		glog.Errorf("Error seeking in statusfile for %s\n", u.Name)
		return UnitStatusUnknown, err
	}
	buf := make([]byte, 32)
	n, err := u.statusFile.Read(buf)
	if err != nil {
		glog.Errorf("Error reading statusfile for %s\n", u.Name)
		return UnitStatusUnknown, err
	}
	s := string(buf[:n])
	err = nil
	var status UnitStatus
	switch s {
	case string(UnitStatusCreated):
		status = UnitStatusCreated
	case string(UnitStatusRunning):
		status = UnitStatusRunning
	case string(UnitStatusFailed):
		status = UnitStatusFailed
	case string(UnitStatusSucceeded):
		status = UnitStatusSucceeded
	default:
		status = UnitStatusUnknown
		err := fmt.Errorf("Invalid status for %s: '%v'\n", u.Name, s)
		glog.Error(err)
	}
	return status, err
}

func (u *Unit) SetStatus(status UnitStatus) error {
	glog.Infof("Updating status of unit '%s' to %s\n", u.Name, status)
	_, err := u.statusFile.Seek(0, 0)
	if err != nil {
		glog.Errorf("Error seeking in statusfile for %s\n", u.Name)
		return err
	}
	buf := []byte(status)
	if _, err := u.statusFile.Write(buf); err != nil {
		glog.Errorf("Error updating statusfile for %s\n", u.Name)
		return err
	}
	u.statusFile.Truncate(int64(len(buf)))
	return nil
}

func (u *Unit) runUnitLoop(command, env []string, unitout, uniterr *os.File,
	policy RestartPolicy) (err error) {
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

func RestartPolicyToString(policy RestartPolicy) string {
	pstr := ""
	switch policy {
	case RESTART_POLICY_ALWAYS:
		pstr = "always"
	case RESTART_POLICY_NEVER:
		pstr = "never"
	case RESTART_POLICY_ONFAILURE:
		pstr = "onfailure"
	}
	return pstr
}

func StringToRestartPolicy(pstr string) RestartPolicy {
	policy := RESTART_POLICY_ALWAYS
	switch strings.ToLower(pstr) {
	case "always":
		policy = RESTART_POLICY_ALWAYS
	case "never":
		policy = RESTART_POLICY_NEVER
	case "onfailure":
		policy = RESTART_POLICY_ONFAILURE
	default:
		glog.Warningf("Invalid restart policy %s; using default 'always'\n",
			pstr)
	}
	return policy
}

func (u *Unit) Run(command, env []string, policy RestartPolicy) error {
	rootfs := u.GetRootfs()
	if _, err := os.Stat(rootfs); os.IsNotExist(err) {
		// No chroot package has been deployed for the unit.
		rootfs = ""
	}

	// Open log pipes _before_ chrooting, since the named pipes are outside of
	// the rootfs.
	lp := u.LogPipe
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

	err = u.runUnitLoop(command, env, unitout, uniterr, policy)

	if rootfs != "" {
		unmountSpecial()
	}

	return err
}
