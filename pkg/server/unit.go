package server

import (
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
	"github.com/elotl/itzo/pkg/caps"
	"github.com/elotl/itzo/pkg/mount"
	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
	"github.com/syndtr/gocapability/capability"
	"golang.org/x/sys/unix"
)

const (
	MAX_BACKOFF_TIME   = 5 * time.Minute
	BACKOFF_RESET_TIME = 10 * time.Minute
	CHILD_OOM_SCORE    = 15 // chosen arbitrarily... kernel will adjust this value
)

const defaultPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

var (
	// List of capabilities granted to units by default. We use the same set as
	// Docker and rkt. See
	// https://docs.docker.com/engine/reference/run/#runtime-privilege-and-linux-capabilities
	// and
	// https://github.com/appc/spec/blob/master/spec/ace.md#oslinuxcapabilities-remove-set
	// for more information.
	defaultCapabilities = []string{
		"CAP_AUDIT_WRITE",
		"CAP_CHOWN",
		"CAP_DAC_OVERRIDE",
		"CAP_FOWNER",
		"CAP_FSETID",
		"CAP_KILL",
		"CAP_MKNOD",
		"CAP_NET_BIND_SERVICE",
		"CAP_NET_RAW",
		"CAP_SETFCAP",
		"CAP_SETGID",
		"CAP_SETPCAP",
		"CAP_SETUID",
		"CAP_SYS_CHROOT",
	}
	sleep = time.Sleep // Allow time.Sleep() to be mocked out in tests.
)

// This is part of the config of docker images.
type HealthConfig struct {
	Test        []string      `json:",omitempty"`
	Interval    time.Duration `json:",omitempty"`
	Timeout     time.Duration `json:",omitempty"`
	StartPeriod time.Duration `json:",omitempty"`
	Retries     int           `json:",omitempty"`
}

// This is the main config struct for docker images.
type Config struct {
	Hostname        string
	Domainname      string
	User            string
	AttachStdin     bool
	AttachStdout    bool
	AttachStderr    bool
	ExposedPorts    map[string]struct{} `json:",omitempty"`
	Tty             bool
	OpenStdin       bool
	StdinOnce       bool
	Env             []string
	Cmd             []string
	Healthcheck     *HealthConfig `json:",omitempty"`
	ArgsEscaped     bool          `json:",omitempty"`
	Image           string
	Volumes         map[string]struct{}
	WorkingDir      string
	Entrypoint      []string
	NetworkDisabled bool   `json:",omitempty"`
	MacAddress      string `json:",omitempty"`
	OnBuild         []string
	Labels          map[string]string
	StopSignal      string   `json:",omitempty"`
	StopTimeout     *int     `json:",omitempty"`
	Shell           []string `json:",omitempty"`
}

// This is the combination of the pod's and the unit's security context.
type securityContext struct {
	api.PodSecurityContext `json:"podSecurityContext"`
	api.SecurityContext    `json:"securityContext"`
}

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
	Directory       string
	Name            string
	Image           string
	statusPath      string
	config          *Config
	securityContext *securityContext
	stdinPath       string
	stdinCloser     chan struct{}
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
	err = u.createStdin()
	if err != nil {
		lp.Remove()
		return nil, err
	}
	u.config, err = u.getConfig()
	if err != nil && !os.IsNotExist(err) {
		glog.Warningf("Failed to get unit %s config: %v", name, err)
	}
	u.securityContext, err = u.getSecurityContext()
	if err != nil && !os.IsNotExist(err) {
		glog.Warningf("Failed to get unit %s security context: %v", name, err)
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

func (u *Unit) createStdin() error {
	pipepath := filepath.Join(u.Directory, "unit-stdin")
	err := syscall.Mkfifo(pipepath, 0600)
	if err != nil && !os.IsExist(err) {
		glog.Errorf("Error creating stdin pipe %s: %v", pipepath, err)
		return err
	}
	u.stdinPath = pipepath
	u.stdinCloser = make(chan struct{})
	return nil
}

// This is only used internally to pass in an io.Reader to the process as its
// stdin. We also start a writer so that opening the pipe for reading won't
// block. This writer will be stopped via closeStdin().
func (u *Unit) openStdinReader() (io.ReadCloser, error) {
	go func() {
		wfp, err := os.OpenFile(u.stdinPath, os.O_WRONLY, 0200)
		if err != nil {
			glog.Errorf("Error opening stdin pipe %s: %v", u.stdinPath, err)
		} else {
			defer wfp.Close()
		}
		select {
		case _ = <-u.stdinCloser:
			break
		}
	}()
	fp, err := os.OpenFile(u.stdinPath, os.O_RDONLY, 0400)
	if err != nil {
		glog.Errorf("Error opening stdin pipe %s: %v", u.stdinPath, err)
		return nil, err
	}
	return fp, nil
}

func (u *Unit) OpenStdinWriter() (io.WriteCloser, error) {
	fp, err := os.OpenFile(u.stdinPath, os.O_WRONLY, 0200)
	if err != nil {
		glog.Errorf("Error opening stdin pipe %s: %v", u.stdinPath, err)
		return nil, err
	}
	return fp, nil
}

func (u *Unit) closeStdin() {
	select {
	case u.stdinCloser <- struct{}{}:
	default:
		glog.Warningf("Stdin for unit %s has already been closed", u.Name)
	}
}

func (u *Unit) getSecurityContext() (*securityContext, error) {
	path := filepath.Join(u.Directory, "securityContext")
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			glog.Errorf("Error reading image securityContext for %s\n", u.Name)
		}
		return nil, err
	}
	var sc securityContext
	err = json.Unmarshal(buf, &sc)
	if err != nil {
		glog.Errorf("Error deserializing securityContext '%v' for %s: %v\n",
			buf, u.Name, err)
		return nil, err
	}
	return &sc, nil
}

func (u *Unit) getConfig() (*Config, error) {
	path := filepath.Join(u.Directory, "config")
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			glog.Errorf("Error reading image config for %s\n", u.Name)
		}
		return nil, err
	}
	var config Config
	err = json.Unmarshal(buf, &config)
	if err != nil {
		glog.Errorf("Error deserializing config '%v' for %s: %v\n",
			buf, u.Name, err)
		return nil, err
	}
	return &config, nil
}

func (u *Unit) CreateCommand(command []string, args []string) []string {
	// See
	// https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell
	// for more information on the possible interactions between k8s
	// command/args and docker entrypoint/cmd.
	if len(command) == 0 {
		command = u.config.Entrypoint
		if len(args) == 0 {
			args = u.config.Cmd
		}
	}
	if len(command) == 0 && len(args) == 0 {
		glog.Warningf("No command or entrypoint for unit %s", u.Name)
	}
	return append(command, args...)
}

func (u *Unit) GetEnv() []string {
	return u.config.Env
}

func (u *Unit) GetWorkingDir() string {
	return u.config.WorkingDir
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

func (u *Unit) Destroy() error {
	// You'll need to kill the child process before.
	u.LogPipe.Remove()
	u.closeStdin()
	return os.RemoveAll(u.Directory)
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
	args := []string{
		"-image",
		image,
		"-extractto",
		u.GetRootfs(),
		"-saveconfig",
		filepath.Join(u.Directory, "config"),
	}
	if username != "" {
		args = append(args, []string{"-username", username}...)
	}
	if password != "" {
		args = append(args, []string{"-password", password}...)
	}
	if url != "" {
		args = append(args, []string{"-url", url}...)
	}
	err = runTosi(tp, args...)
	if err != nil {
		return err
	}
	// Make sure that there is a working resolv.conf inside the unit.
	err = u.copyFileFromHost("/etc/resolv.conf", true)
	if err != nil {
		return err
	}
	return nil
}

func (u *Unit) getUser() (uint32, uint32, []uint32, string, error) {
	var err error
	uid := uint32(0)
	gid := uint32(0)
	groups := make([]uint32, 0)
	homedir := "/"
	// Check the image config for user/group.
	if u.config.User != "" {
		oul := &util.OsUserLookup{}
		uid, gid, homedir, err = util.LookupUser(u.config.User, oul)
		if err != nil {
			return 0, 0, nil, "", err
		}
	}
	// Next, pod security context for uid/groups.
	// TODO
	// Last, unit security context for uid.
	// TODO
	return uid, gid, groups, homedir, nil
}

func (u *Unit) copyFileFromHost(hostpath string, overwrite bool) error {
	dpath := filepath.Join(u.GetRootfs(), filepath.Dir(hostpath))
	if _, err := os.Stat(dpath); os.IsNotExist(err) {
		glog.Infof("Creating directory %s", dpath)
		if err := os.MkdirAll(dpath, 0755); err != nil {
			glog.Errorf("Could not create new directory %s: %v", dpath, err)
			return err
		}
	}
	fpath := filepath.Join(u.GetRootfs(), hostpath)
	if _, err := os.Stat(fpath); os.IsNotExist(err) || overwrite {
		glog.Infof("Copying %s from host to %s", hostpath, fpath)
		if err := copyFile(hostpath, fpath); err != nil {
			glog.Errorf("copyFile() %s to %s: %v", hostpath, fpath, err)
			return err
		}
	}
	return nil
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
		glog.Errorf("Error serializing status for %s: %v\n", u.Name, err)
		return err
	}
	if err := ioutil.WriteFile(u.statusPath, buf, 0600); err != nil {
		glog.Errorf("Error updating statusfile for %s: %v\n", u.Name, err)
		return err
	}
	return nil
}

func maybeBackOff(err error, command []string, backoff *time.Duration, runningTime time.Duration) {
	if err == nil || runningTime >= BACKOFF_RESET_TIME {
		// Reset backoff.
		*backoff = 1 * time.Second
	} else {
		*backoff *= 2
		if *backoff > MAX_BACKOFF_TIME {
			*backoff = MAX_BACKOFF_TIME
		}
	}
	glog.Infof("Waiting for %v before starting %v again", *backoff, command)
	sleep(*backoff)
}

func (u *Unit) runUnitLoop(command, env, caplist []string, uid, gid uint32, groups []uint32, unitin io.Reader, unitout, uniterr io.Writer, policy api.RestartPolicy) (err error) {
	backoff := 1 * time.Second
	restarts := -1
	for {
		restarts++
		start := time.Now()
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Env = env
		cmd.Stdin = unitin
		cmd.Stdout = unitout
		cmd.Stderr = uniterr
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		if len(caplist) > 0 {
			err := u.setCapabilities(caplist)
			if err != nil {
				u.setStateToStartFailure(err)
				glog.Errorf("Setting capabilities %v: %v", caplist, err)
				maybeBackOff(err, command, &backoff, 0*time.Second)
				continue
			}
			cmd.SysProcAttr.AmbientCaps = mapUintptrCapabilities(caplist)
		}
		if uid > 0 || gid > 0 || groups != nil {
			cmd.SysProcAttr.Credential = &syscall.Credential{
				Uid:    uid,
				Gid:    gid,
				Groups: groups,
			}
		}

		err = cmd.Start()
		if err != nil {
			// Start() failed, it is either an error looking up the executable,
			// or a resource allocation problem.
			u.SetState(api.UnitState{
				Waiting: &api.UnitStateWaiting{
					StartFailure: true,
					Reason:       err.Error(),
				},
			}, &restarts)
			glog.Errorf("Start() %v: %v", command, err)
			maybeBackOff(err, command, &backoff, 0*time.Second)
			continue
		}
		u.SetState(api.UnitState{
			Running: &api.UnitStateRunning{
				StartedAt: api.Now(),
			},
		}, &restarts)
		if cmd.Process != nil {
			glog.Infof("Command %v running as pid %d", command, cmd.Process.Pid)
			err := util.SetOOMScore(cmd.Process.Pid, CHILD_OOM_SCORE)
			if err != nil {
				glog.Warningf("Error resetting oom score for pid %v: %s",
					cmd.Process.Pid, err)
			}
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
		maybeBackOff(procErr, command, &backoff, d)
	}
}

func createDir(dir string, uid, gid int) error {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		glog.Errorf("Failed to create directory %s: %v", dir, err)
		return err
	}
	err = os.Chown(dir, uid, gid)
	if err != nil {
		glog.Errorf("Failed to change UID/GID of directory %s to %d/%d: %v",
			dir, uid, gid, err)
		return err
	}
	return nil
}

func changeToWorkdir(workingdir string, uid, gid uint32) error {
	// Workingdir might not exist, try to create it first.
	os.MkdirAll(workingdir, 0755)
	err := os.Chdir(workingdir)
	if err != nil {
		glog.Errorf("Failed to change to working directory %s: %v",
			workingdir, err)
		return err
	}
	if uid != 0 || gid != 0 {
		err = os.Chown(workingdir, int(uid), int(gid))
		if err != nil {
			glog.Errorf("Failed to chown workingdir %s to %d/%d: %v",
				workingdir, uid, gid, err)
			return err
		}
	}
	return nil
}

func (u *Unit) setStateToStartFailure(err error) {
	serr := fmt.Sprintf("Failed to start: %v", err)
	u.SetState(api.UnitState{
		Waiting: &api.UnitStateWaiting{
			Reason:       serr,
			StartFailure: true,
		},
	}, nil)
}

func mapCapabilities(keys []string) []capability.Cap {
	cs := make([]capability.Cap, 0)
	for _, key := range keys {
		v := caps.GetCapability(key)
		if v != nil {
			cs = append(cs, v.Value)
		}
	}
	return cs
}

func mapUintptrCapabilities(keys []string) []uintptr {
	cs := mapCapabilities(keys)
	uintptrCs := make([]uintptr, len(cs))
	for i, c := range cs {
		uintptrCs[i] = uintptr(c)
	}
	return uintptrCs
}

func (u *Unit) getCapabilities() ([]string, error) {
	addCaps := []string{}
	dropCaps := []string{}
	if u.securityContext != nil && u.securityContext.SecurityContext.Capabilities != nil {
		addCaps = u.securityContext.SecurityContext.Capabilities.Add
		dropCaps = u.securityContext.SecurityContext.Capabilities.Drop
	}
	capStringList, err := caps.TweakCapabilities(
		defaultCapabilities, addCaps, dropCaps, nil, false)
	if err != nil {
		return nil, err
	}
	return capStringList, nil
}

func (u *Unit) setCapabilities(capStringList []string) error {
	c, err := capability.NewPid2(0)
	if err != nil {
		return err
	}
	err = c.Load()
	if err != nil {
		return err
	}
	capList := mapCapabilities(capStringList)
	c.Set(capability.CAPS|capability.BOUNDS|capability.AMBIENT, capList...)
	if err := c.Apply(capability.CAPS | capability.BOUNDS | capability.AMBIENT); err != nil {
		return err
	}
	if err := unix.Prctl(unix.PR_SET_KEEPCAPS, 1, 0, 0, 0); err != nil {
		return err
	}
	return nil
}

func (u *Unit) Run(podname string, command, env []string, workingdir string, policy api.RestartPolicy, mounter mount.Mounter) error {
	u.SetState(api.UnitState{
		Waiting: &api.UnitStateWaiting{
			Reason: "starting",
		},
	}, nil)

	rootfs := u.GetRootfs()
	if _, err := os.Stat(rootfs); os.IsNotExist(err) {
		// No chroot package has been deployed for the unit.
		rootfs = ""
		glog.Errorf("No rootfs found for %s; not chrooting", u.Name)
	}

	// Open log pipes _before_ chrooting, since the named pipes are outside of
	// the rootfs.
	lp := u.LogPipe
	helperout, err := lp.OpenWriter(PIPE_HELPER_OUT, true)
	if err != nil {
		lp.Remove()
		u.setStateToStartFailure(err)
		return err
	}
	defer helperout.Close()
	unitout, err := lp.OpenWriter(PIPE_UNIT_STDOUT, false)
	if err != nil {
		lp.Remove()
		u.setStateToStartFailure(err)
		return err
	}
	defer unitout.Close()
	uniterr, err := lp.OpenWriter(PIPE_UNIT_STDERR, false)
	if err != nil {
		lp.Remove()
		u.setStateToStartFailure(err)
		return err
	}
	defer uniterr.Close()
	unitin, err := u.openStdinReader()
	if err != nil {
		u.setStateToStartFailure(err)
		return err
	}
	defer unitin.Close()

	if rootfs != "" {
		oldrootfs := fmt.Sprintf("%s/.oldrootfs", rootfs)

		if err := mounter.BindMount(rootfs, rootfs); err != nil {
			glog.Errorf("Mount() %s: %v", rootfs, err)
			u.setStateToStartFailure(err)
			return err
		}
		// Bind mount statusfile into the chroot. Note: both the source and the
		// destination files need to exist, otherwise the bind mount will fail.
		statussrc := filepath.Join(u.statusPath)
		err := ensureFileExists(statussrc)
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
			u.setStateToStartFailure(err)
			return err
		}
		if err := os.MkdirAll(oldrootfs, 0700); err != nil {
			glog.Errorf("MkdirAll() %s: %v", oldrootfs, err)
			u.setStateToStartFailure(err)
			return err
		}
		if err := mounter.PivotRoot(rootfs, oldrootfs); err != nil {
			glog.Errorf("PivotRoot() %s %s: %v", rootfs, oldrootfs, err)
			u.setStateToStartFailure(err)
			return err
		}
		if err := os.Chdir("/"); err != nil {
			glog.Errorf("Chdir() /: %v", err)
			u.setStateToStartFailure(err)
			return err
		}
		// Mark the old root mount sharing as private so we don't
		// unmount any volumes living in the root that are shared
		// between namespaces as emptyDirs when we unmount the old
		// root.
		shareFlags := uintptr(syscall.MS_PRIVATE | syscall.MS_REC)
		if err := mount.ShareMount("/.oldrootfs", shareFlags); err != nil {
			glog.Errorf("ShareMount(%s, private): %v", oldrootfs, err)
			u.setStateToStartFailure(err)
			return err
		}
		if err := mounter.Unmount("/.oldrootfs"); err != nil {
			glog.Errorf("Unmount() %s: %v", oldrootfs, err)
			u.setStateToStartFailure(err)
			return err
		}
		os.Remove("/.oldrootfs")

		if err := mounter.MountSpecial(); err != nil {
			glog.Errorf("mountSpecial(): %v", err)
			u.setStateToStartFailure(err)
			return err
		}
		defer mounter.UnmountSpecial()
		u.statusPath = "/status"
	}

	uid, gid, groups, homedir, err := u.getUser()
	if err != nil {
		u.setStateToStartFailure(err)
		return err
	}

	// Make user HOME, HOSTNAME, PATH and TERM are set (same variables Docker
	// ensures are set). See
	// https://docs.docker.com/v17.09/engine/reference/run/#env-environment-variables
	// for more information.
	if podname != "" {
		err = syscall.Sethostname([]byte(podname))
		if err != nil {
			glog.Errorf("Failed to set hostname to %s: %v", podname, err)
			u.setStateToStartFailure(err)
			return err
		}
		env = util.AddToEnvList(env, "HOSTNAME", podname, true)
	}
	env = util.AddToEnvList(env, "TERM", "xterm", false)
	env = util.AddToEnvList(env, "HOME", homedir, false)
	env = util.AddToEnvList(env, "PATH", defaultPath, false)

	err = os.Chmod("/", 0755)
	if err != nil {
		glog.Errorf("Failed to chmod / to 0755: %v", err)
		u.setStateToStartFailure(err)
		return err
	}

	for vol, _ := range u.config.Volumes {
		err = createDir(vol, int(uid), int(gid))
		if err != nil {
			u.setStateToStartFailure(err)
			return err
		}
	}

	if workingdir != "" {
		err = changeToWorkdir(workingdir, uid, gid)
		if err != nil {
			u.setStateToStartFailure(err)
			return err
		}
	}

	caplist, err := u.getCapabilities()
	if err != nil {
		u.setStateToStartFailure(err)
		return err
	}

	return u.runUnitLoop(command, env, caplist, uid, gid, groups, unitin, unitout, uniterr, policy)
}
