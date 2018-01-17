package server

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
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
	statusPath string
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

func IsUnitExist(rootdir, name string) bool {
	f, err := os.Open(filepath.Join(rootdir, name))
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func OpenUnit(rootdir, name string) (*Unit, error) {
	glog.Infof("Creating new unit '%s' in %s\n", name, rootdir)
	directory := filepath.Join(rootdir, name)
	// Make sure unit directory exists.
	if err := os.MkdirAll(directory, 0700); err != nil {
		glog.Errorf("Error reating unit '%s': %v\n", name, err)
		return nil, err
	}
	lp, err := NewLogPipe(directory)
	if err != nil {
		glog.Errorf("Error creating logpipes for unit '%s': %v\n", name, err)
		return nil, err
	}
	u := Unit{
		LogPipe:    lp,
		Directory:  directory,
		Name:       name,
		statusPath: filepath.Join(directory, "status"),
	}
	return &u, nil
}

func (u *Unit) Close() {
	// No-op for now.
}

func (u *Unit) GetRootfs() string {
	return filepath.Join(u.Directory, "ROOTFS")
}

func (u *Unit) GetStatus() (UnitStatus, error) {
	buf, err := ioutil.ReadFile(u.statusPath)
	if err != nil {
		if os.IsNotExist(err) {
			return UnitStatusUnknown, nil
		}
		glog.Errorf("Error reading statusfile for %s\n", u.Name)
		return UnitStatusUnknown, err
	}
	err = nil
	var status UnitStatus
	switch string(buf) {
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
		err := fmt.Errorf("Invalid status for %s: '%v'\n", u.Name, buf)
		glog.Error(err)
	}
	return status, err
}

func (u *Unit) SetStatus(status UnitStatus) error {
	glog.Infof("Updating status of unit '%s' to %s\n", u.Name, status)
	buf := []byte(status)
	if err := ioutil.WriteFile(u.statusPath, buf, 0600); err != nil {
		glog.Errorf("Error updating statusfile for %s\n", u.Name)
		return err
	}
	return nil
}

func (u *Unit) runUnitLoop(command, env []string, unitout, uniterr *os.File,
	policy RestartPolicy) (err error) {
	u.SetStatus(UnitStatusRunning)
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
			u.SetStatus(UnitStatusFailed)
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
			if err != nil {
				u.SetStatus(UnitStatusFailed)
			} else {
				u.SetStatus(UnitStatusSucceeded)
			}
			return err
		case RESTART_POLICY_ONFAILURE:
			if err == nil {
				u.SetStatus(UnitStatusSucceeded)
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
	u.SetStatus(UnitStatusCreated)

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
		// Bind mount statusfile into the chroot. Note: both the source and the
		// destination files need to exist, otherwise the bind mount will fail.
		statussrc := filepath.Join(u.statusPath)
		ensureFileExists(statussrc)
		statusdst := filepath.Join(u.GetRootfs(), "status")
		ensureFileExists(statusdst)
		if err := syscall.Mount(statussrc, statusdst, "", syscall.MS_BIND, ""); err != nil {
			glog.Errorf("Mount() statusfile: %v", err)
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
		u.statusPath = "/status"
	}

	err = u.runUnitLoop(command, env, unitout, uniterr, policy)

	if rootfs != "" {
		unmountSpecial()
	}

	return err
}

type Link struct {
	dst      string
	src      string
	linktype byte
	mode     os.FileMode
	uid      int
	gid      int
}

func (u *Unit) DeployPackage(filename string) (err error) {
	rootfs := u.GetRootfs()
	err = os.MkdirAll(rootfs, 0700)
	if err != nil {
		glog.Errorln("creating rootfs", rootfs, ":", err)
		return err
	}

	f, err := os.Open(filename)
	if err != nil {
		glog.Errorln("opening package file:", err)
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		glog.Errorln("uncompressing package:", err)
		return err
	}
	defer gzr.Close()

	var links []Link

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			glog.Errorln("extracting package:", err)
			return err
		}

		name := header.Name
		if name == "ROOTFS" {
			continue
		}
		if len(name) < 7 || name[:7] != "ROOTFS/" {
			glog.Warningln("file outside of ROOTFS in package:", name)
			continue
		}
		name = filepath.Join(rootfs, name[7:])

		switch header.Typeflag {
		case tar.TypeDir: // directory
			glog.Infoln("d", name)
			os.Mkdir(name, os.FileMode(header.Mode))
		case tar.TypeReg: // regular file
			glog.Infoln("f", name)
			data := make([]byte, header.Size)
			read_so_far := int64(0)
			for read_so_far < header.Size {
				n, err := tr.Read(data[read_so_far:])
				if err != nil && err != io.EOF {
					glog.Errorln("extracting", name, ":", err)
					return err
				}
				read_so_far += int64(n)
			}
			if read_so_far != header.Size {
				glog.Errorf("f %s error: read %d bytes, but size is %d bytes", name, read_so_far, header.Size)
			}
			ioutil.WriteFile(name, data, os.FileMode(header.Mode))
		case tar.TypeLink, tar.TypeSymlink:
			linkname := header.Linkname
			if len(linkname) >= 7 && linkname[:7] == "ROOTFS/" {
				linkname = filepath.Join(rootfs, linkname[7:])
			}
			// Links might point to files or directories that have not been
			// extracted from the tarball yet. Create them after going through
			// all entries in the tarball.
			links = append(links, Link{linkname, name, header.Typeflag, os.FileMode(header.Mode), header.Uid, header.Gid})
			continue
		default:
			glog.Warningf("unknown type while untaring: %d", header.Typeflag)
			continue
		}
		err = os.Chown(name, header.Uid, header.Gid)
		if err != nil {
			glog.Warningf("warning: chown %s type %d uid %d gid %d: %v", name, header.Typeflag, header.Uid, header.Gid, err)
		}
	}

	for _, link := range links {
		os.Remove(link.src) // Remove link in case it exists.
		if link.linktype == tar.TypeSymlink {
			glog.Infoln("s", link.src)
			err = os.Symlink(link.dst, link.src)
			if err != nil {
				glog.Errorf("creating symlink %s -> %s: %v", link.src, link.dst, err)
				return err
			}
			err = os.Lchown(link.src, link.uid, link.gid)
			if err != nil {
				glog.Warningf("warning: chown symlink %s uid %d gid %d: %v", link.src, link.uid, link.gid, err)
			}
		}
		if link.linktype == tar.TypeLink {
			glog.Infoln("h", link.src)
			err = os.Link(link.dst, link.src)
			if err != nil {
				glog.Errorf("creating hardlink %s -> %s: %v", link.src, link.dst, err)
				return err
			}
			err = os.Chmod(link.src, link.mode)
			if err != nil {
				glog.Errorf("chmod hardlink %s %d: %v", link.src, link.mode, err)
				return err
			}
			err = os.Chown(link.src, link.uid, link.gid)
			if err != nil {
				glog.Warningf("warning: chown hardlink %s uid %d gid %d: %v", link.src, link.uid, link.gid, err)
			}
		}
	}

	return nil
}
