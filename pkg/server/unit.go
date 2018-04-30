package server

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/mount"
	"github.com/golang/glog"
)

const (
	MAX_BACKOFF_TIME = 5 * time.Minute
)

func makeStillCreatingStatus(name, image, reason string) *api.UnitStatus {
	return &api.UnitStatus{
		Name: name,
		State: api.UnitState{
			Waiting: &api.UnitStateWaiting{
				Reason: reason,
			},
		},
		RestartCount: 0,
		Image:        image,
	}
}

type Unit struct {
	*LogPipe
	Directory  string
	Name       string
	Image      string
	statusPath string
}

func IsUnitExist(rootdir, name string) bool {
	if len(name) == 0 {
		return false
	}
	f, err := os.Open(filepath.Join(rootdir, name))
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func OpenUnit(rootdir, name string) (*Unit, error) {
	directory := filepath.Join(rootdir, name)
	// Make sure unit directory exists.
	if err := os.MkdirAll(directory, 0700); err != nil {
		glog.Errorf("Error creating unit '%s': %v\n", name, err)
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
	// We need to get the image, that's saved in the status
	s, err := u.GetStatus()
	if err != nil {
		glog.Warningf("Error getting unit %s status: %v", name, err)
	} else {
		u.Image = s.Image
	}
	return &u, nil
}

func (u *Unit) SetImage(image string) error {
	u.Image = image
	status, err := u.GetStatus()
	if err != nil {
		return err
	}
	status.Image = u.Image
	buf, err := json.Marshal(status)
	if err != nil {
		glog.Errorf("Error serializing status for %s\n", u.Name)
		return err
	}
	if err := ioutil.WriteFile(u.statusPath, buf, 0600); err != nil {
		glog.Errorf("Error updating statusfile for %s\n", u.Name)
		return err
	}
	return nil
}

func (u *Unit) Close() {
	// No-op for now.
}

func (u *Unit) Destroy() {
	// you'll need to kill the child process before
	u.LogPipe.Remove()
	os.RemoveAll(u.Directory)
}

func (u *Unit) GetRootfs() string {
	return filepath.Join(u.Directory, "ROOTFS")
}

func (u *Unit) PullAndExtractImage(image, url, username, password string) error {
	if u.Image != "" {
		glog.Warningf("Unit %s has already pulled image %s", u.Name, u.Image)
	}
	glog.Infof("Unit %s pulling image %s", u.Name, image)
	err := u.SetImage(image)
	if err != nil {
		return fmt.Errorf("Error setting image for unit: %v", err)
	}
	tp, err := exec.LookPath(TOSI_PRG)
	if err != nil {
		tp = "/tmp/tosiprg"
		err = downloadTosi(tp)
	}
	if err != nil {
		return err
	}
	args := []string{"-image", image, "-extractto", u.GetRootfs()}
	if username != "" {
		args = append(args, []string{"-username", username}...)
	}
	if password != "" {
		args = append(args, []string{"-password", password}...)
	}
	if url != "" {
		args = append(args, []string{"-url", url}...)
	}
	return runTosi(tp, args...)
}

func (u *Unit) GetStatus() (*api.UnitStatus, error) {
	buf, err := ioutil.ReadFile(u.statusPath)
	if err != nil {
		if os.IsNotExist(err) {
			return makeStillCreatingStatus(u.Name, u.Image, "Unit creating"), nil
		}
		glog.Errorf("Error reading statusfile for %s\n", u.Name)
		return nil, err
	}
	var status api.UnitStatus
	err = json.Unmarshal(buf, &status)
	return &status, err
}

func (u *Unit) SetState(state api.UnitState, restarts *int) error {
	// Check current status, and update status.State. Name and Image are
	// immutable, and RestartCount is kept up to date automatically here.
	// pass in a nil pointer to restarts to not update that value
	status, err := u.GetStatus()
	if err != nil {
		glog.Errorf("Error getting current status for %s\n", u.Name)
		return err
	}
	glog.Infof("Updating state of unit '%s' to %v\n", u.Name, state)
	status.State = state
	if restarts != nil && *restarts >= 0 {
		status.RestartCount = int32(*restarts)
	}
	buf, err := json.Marshal(status)
	if err != nil {
		glog.Errorf("Error serializing status for %s\n", u.Name)
		return err
	}
	if err := ioutil.WriteFile(u.statusPath, buf, 0600); err != nil {
		glog.Errorf("Error updating statusfile for %s\n", u.Name)
		return err
	}
	return nil
}

func maybeBackOff(err error, command []string, backoff *time.Duration) {
	if err != nil {
		*backoff *= 2
		if *backoff > MAX_BACKOFF_TIME {
			*backoff = MAX_BACKOFF_TIME
		}
	} else {
		// Reset backoff.
		*backoff = 1 * time.Second
	}
	glog.Infof("Waiting for %v before starting %v again", *backoff, command)
	time.Sleep(*backoff)
}

func (u *Unit) runUnitLoop(command, env []string, unitout, uniterr *os.File, policy api.RestartPolicy) (err error) {
	backoff := 1 * time.Second
	restarts := -1
	for {
		restarts++
		start := time.Now()
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Env = env
		cmd.Stdout = unitout
		cmd.Stderr = uniterr

		err = cmd.Start()
		if err != nil {
			// Start() failed, it is either an error looking up the executable,
			// or a resource allocation problem.
			u.SetState(api.UnitState{
				Waiting: &api.UnitStateWaiting{
					LaunchFailure: true,
					Reason:        err.Error(),
				},
			}, &restarts)
			glog.Errorf("Start() %v: %v", command, err)
			maybeBackOff(err, command, &backoff)
			continue
		}
		u.SetState(api.UnitState{
			Running: &api.UnitStateRunning{
				StartedAt: api.Now(),
			},
		}, &restarts)
		if cmd.Process != nil {
			glog.Infof("Command %v running as pid %d", command, cmd.Process.Pid)
		} else {
			glog.Warningf("cmd has nil process: %#v", cmd)
		}

		exitCode := 0

		procErr := cmd.Wait()
		d := time.Since(start)
		failure := false
		if procErr != nil {
			failure = true
			foundRc := false
			if exiterr, ok := procErr.(*exec.ExitError); ok {
				if ws, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					foundRc = true
					exitCode = ws.ExitStatus()
					glog.Infof("Command %v pid %d exited with %d after %.2fs",
						command, cmd.Process.Pid, exitCode, d.Seconds())
				}
			}
			if !foundRc {
				glog.Infof("Command %v pid %d exited with %v after %.2fs",
					command, cmd.Process.Pid, procErr, d.Seconds())
			}
		} else {
			glog.Infof("Command %v pid %d exited with 0 after %.2fs",
				command, cmd.Process.Pid, d.Seconds())
		}

		if policy == api.RestartPolicyAlways ||
			(policy == api.RestartPolicyOnFailure && failure) {
			// We never mark a unit as terminated in this state,
			// we just return it to waiting and wait for it to
			// be run again
			u.SetState(api.UnitState{
				Waiting: &api.UnitStateWaiting{
					Reason: fmt.Sprintf(
						"Waiting for unit restart, last exit code: %d",
						exitCode),
				},
			}, &restarts)
		} else {
			// Game over, man!
			u.SetState(api.UnitState{
				Terminated: &api.UnitStateTerminated{
					ExitCode:   int32(exitCode),
					FinishedAt: api.Now(),
				},
			}, &restarts)
			return procErr
		}
		maybeBackOff(procErr, command, &backoff)
	}
}

func (u *Unit) Run(command, env []string, policy api.RestartPolicy, mounter mount.Mounter) error {
	u.SetState(api.UnitState{
		Waiting: &api.UnitStateWaiting{
			Reason: "starting",
		},
	}, nil)

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

		if err := mounter.BindMount(rootfs, rootfs); err != nil {
			glog.Errorf("Mount() %s: %v", rootfs, err)
			return err
		}
		// Bind mount statusfile into the chroot. Note: both the source and the
		// destination files need to exist, otherwise the bind mount will fail.
		statussrc := filepath.Join(u.statusPath)
		err = ensureFileExists(statussrc)
		if err != nil {
			glog.Errorln("error creating status file #1")
		}
		statusdst := filepath.Join(u.GetRootfs(), "status")
		err = ensureFileExists(statusdst)
		if err != nil {
			glog.Errorln("error creating status file #2")
		}
		if err := mounter.BindMount(statussrc, statusdst); err != nil {
			glog.Errorf("Mount() statusfile: %v", err)
			return err
		}
		if err := os.MkdirAll(oldrootfs, 0700); err != nil {
			glog.Errorf("MkdirAll() %s: %v", oldrootfs, err)
			return err
		}
		if err := mounter.PivotRoot(rootfs, oldrootfs); err != nil {
			glog.Errorf("PivotRoot() %s %s: %v", rootfs, oldrootfs, err)
			return err
		}
		if err := os.Chdir("/"); err != nil {
			glog.Errorf("Chdir() /: %v", err)
			return err
		}
		if err := mounter.Unmount("/.oldrootfs"); err != nil {
			glog.Errorf("Unmount() %s: %v", oldrootfs, err)
			return err
		}
		os.Remove("/.oldrootfs")
		if err := mounter.MountSpecial(); err != nil {
			glog.Errorf("mountSpecial(): %v", err)
			return err
		}
		u.statusPath = "/status"
	}

	err = u.runUnitLoop(command, env, unitout, uniterr, policy)

	if rootfs != "" {
		mounter.UnmountSpecial()
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
