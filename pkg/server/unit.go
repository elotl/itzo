package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/cgroups"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/caps"
	"github.com/elotl/itzo/pkg/mount"
	"github.com/elotl/itzo/pkg/net"
	"github.com/elotl/itzo/pkg/prober"
	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
	sysctl "github.com/lorenzosaino/go-sysctl"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/syndtr/gocapability/capability"
	"golang.org/x/sys/unix"
)

const (
	MAX_BACKOFF_TIME                     = 5 * time.Minute
	BACKOFF_RESET_TIME                   = 10 * time.Minute
	CHILD_OOM_SCORE                      = 15 // chosen arbitrarily... kernel will adjust this value
	MAX_HOSTNAME_LEN                     = 63
	MaxContainerTerminationMessageLength = 1024 * 4
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

type UnitConfig struct {
	api.PodSecurityContext   `json:"podSecurityContext"`
	api.SecurityContext      `json:"securityContext"`
	StartupProbe             *api.Probe `json:",omitempty`
	ReadinessProbe           *api.Probe `json:",omitempty`
	LivenessProbe            *api.Probe `json:",omitempty`
	TerminationMessagePolicy api.TerminationMessagePolicy
	TerminationMessagePath   string
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
	Directory   string
	Name        string
	Image       string
	statusPath  string
	config      *Config
	unitConfig  UnitConfig
	stdinPath   string
	stdinCloser chan struct{}
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
	u.unitConfig, err = u.getUnitConfig()
	if err != nil && !os.IsNotExist(err) {
		glog.Warningf("Failed to get unit %s configuration: %v", name, err)
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

func (u *Unit) getUnitConfig() (UnitConfig, error) {
	path := filepath.Join(u.Directory, "unitConfig")
	buf, err := ioutil.ReadFile(path)
	var uc UnitConfig
	if err != nil {
		if !os.IsNotExist(err) {
			glog.Errorf("Error reading unit file %s for %q", path, u.Name)
		}
		return uc, err
	}

	err = json.Unmarshal(buf, &uc)
	if err != nil {
		glog.Errorf("Error deserializing %s '%v' for %q: %v",
			path, buf, u.Name, err)
		return uc, err
	}
	return uc, nil
}

func (u *Unit) SaveUnitConfig(unitConfig UnitConfig) error {
	filename := "unitConfig"
	path := filepath.Join(u.Directory, filename)
	buf, err := json.Marshal(&unitConfig)
	if err != nil {
		glog.Errorf("Error serializing %s '%v' for %q: %v",
			filename, buf, u.Name, err)
		return err
	}
	err = ioutil.WriteFile(path, buf, 0755)
	if err != nil {
		if !os.IsNotExist(err) {
			glog.Errorf("Error writing %s for %s\n", filename, u.Name)
		}
		return err
	}
	return nil
}

func (u *Unit) getConfig() (*Config, error) {
	path := filepath.Join(u.Directory, "config")
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			glog.Errorf("Error reading config for %s\n", u.Name)
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
	if len(command) == 0 && u.config != nil {
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
	if u.config == nil {
		return nil
	}
	return u.config.Env
}

func (u *Unit) GetWorkingDir() string {
	if u.config == nil {
		return ""
	}
	return u.config.WorkingDir
}

func (u *Unit) SetImage(image string) error {
	u.Image = image
	status, err := u.GetStatus()
	if err != nil {
		return err
	}
	status.Image = u.Image
	return u.SetStatus(status)
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

func (u *Unit) GetUser(lookup util.UserLookup) (uid, gid uint32, groups []uint32, homedir string, err error) {
	homedir = "/"
	// Check the image config for user/group.
	if u.config != nil && u.config.User != "" {
		uid, gid, homedir, err = util.LookupUser(u.config.User, lookup)
		if err != nil {
			return 0, 0, nil, "", err
		}
	}

	// Next, pod security context for uid/groups.
	if u.unitConfig.PodSecurityContext.RunAsUser != nil {
		uid = uint32(*u.unitConfig.PodSecurityContext.RunAsUser)
	}
	if u.unitConfig.PodSecurityContext.RunAsGroup != nil {
		gid = uint32(*u.unitConfig.PodSecurityContext.RunAsGroup)
	}
	if len(u.unitConfig.PodSecurityContext.SupplementalGroups) > 0 {
		suppGroups := u.unitConfig.PodSecurityContext.SupplementalGroups
		groups = make([]uint32, len(suppGroups))
		for i, g := range suppGroups {
			groups[i] = uint32(g)
		}
	}
	// Last, unit security context for uid.
	if u.unitConfig.SecurityContext.RunAsUser != nil {
		uid = uint32(*u.unitConfig.SecurityContext.RunAsUser)
	}
	if u.unitConfig.SecurityContext.RunAsGroup != nil {
		gid = uint32(*u.unitConfig.SecurityContext.RunAsGroup)
	}
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

func (u *Unit) SetStatus(status *api.UnitStatus) error {
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

func (u *Unit) UpdateStatusAttr(ready, started *bool) error {
	status, err := u.GetStatus()
	if err != nil {
		glog.Errorf("Error getting current status for %s\n", u.Name)
		return err
	}
	if ready != nil {
		status.Ready = *ready
	}
	if started != nil {
		status.Started = started
	}
	return u.SetStatus(status)
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
	if status.State.Terminated != nil {
		status.LastTerminationState = state
	}
	if restarts != nil && *restarts >= 0 {
		status.RestartCount = int32(*restarts)
	}
	return u.SetStatus(status)
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

func (u *Unit) runUnitLoop(command, caplist []string, uid, gid uint32, groups []uint32, unitin io.Reader, unitout, uniterr io.Writer, policy api.RestartPolicy) (err error) {
	falseval := false
	backoff := 1 * time.Second
	restarts := -1
	for {
		restarts++
		startTime := time.Now()
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Env = os.Environ()
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
		u.UpdateStatusAttr(&falseval, &falseval)
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
		cmdErr, probeErr := u.watchRunningCmd(cmd, u.unitConfig.StartupProbe, u.unitConfig.ReadinessProbe, u.unitConfig.LivenessProbe)
		keepGoing := u.handleCmdCleanup(cmd, cmdErr, probeErr, policy, startTime)
		if !keepGoing {
			return cmdErr
		}
		maybeBackOff(cmdErr, command, &backoff, time.Since(startTime))
	}
}

func (u *Unit) watchRunningCmd(cmd *exec.Cmd, startupProbe, readinessProbe, livenessProbe *api.Probe) (error, error) {
	cmdDoneChan := waitForCmd(cmd)
	if startupProbe != nil {
		startupWorker := prober.NewWorker(u.Name, prober.Startup, startupProbe)
		startupWorker.Start()
		defer startupWorker.Stop()
	waitForStarted:
		for {
			select {
			case cmdErr := <-cmdDoneChan:
				return cmdErr, nil
			case startupResult := <-startupWorker.Results():
				if startupResult == prober.Failure {
					glog.Warningln("startup probe failed")
					return nil, fmt.Errorf("startup probe failed")
				} else if startupResult == prober.Success {
					break waitForStarted
				}
			}
		}
		startupWorker.Stop()
	}

	isReady := readinessProbe == nil
	isStarted := true
	u.UpdateStatusAttr(&isReady, &isStarted)

	livenessWorker := prober.NewWorker(u.Name, prober.Liveness, livenessProbe)
	livenessWorker.Start()
	defer livenessWorker.Stop()
	readinessWorker := prober.NewWorker(u.Name, prober.Readiness, readinessProbe)
	readinessWorker.Start()
	defer readinessWorker.Stop()
	for {
		select {
		case cmdErr := <-cmdDoneChan:
			return cmdErr, nil
		case livenessResult := <-livenessWorker.Results():
			if livenessResult == prober.Failure {
				glog.Warningln("liveness probe failed")
				return nil, fmt.Errorf("liveness probe failed")
			}
		case readinessResult := <-readinessWorker.Results():
			// this will never fire if we don't have a readiness probe
			ready := readinessResult == prober.Success
			u.UpdateStatusAttr(&ready, nil)
		}
	}
	return nil, nil
}

func waitForCmd(cmd *exec.Cmd) chan error {
	// prevent leaking a goroutine by buffering the channel
	doneChan := make(chan error, 1)
	go func() {
		procErr := cmd.Wait()
		doneChan <- procErr
	}()
	return doneChan
}

func (u *Unit) handleCmdCleanup(cmd *exec.Cmd, cmdErr, probeErr error, policy api.RestartPolicy, startTime time.Time) (keepGoing bool) {
	keepGoing = true
	d := time.Since(startTime)
	failure := false
	exitCode := 0
	fullCmd := append([]string{cmd.Path}, cmd.Args...)
	reason := ""
	if cmdErr != nil {
		failure = true
		foundRc := false
		reason = "Error"
		if exiterr, ok := cmdErr.(*exec.ExitError); ok {
			if ws, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				foundRc = true
				exitCode = ws.ExitStatus()
				glog.Infof("Command %v pid %d exited with %d after %.2fs",
					fullCmd, cmd.Process.Pid, exitCode, d.Seconds())
			}
		}
		if !foundRc {
			glog.Infof("Command %v pid %d exited with %v after %.2fs",
				fullCmd, cmd.Process.Pid, cmdErr, d.Seconds())
		}
	} else if probeErr != nil {
		glog.Infof("Command %s saw a probe error %s after %.2fs",
			fullCmd, probeErr.Error(), d.Seconds())
		//
		// Todo: this should abide by the unit's terminationGracePeriod
		//
		reason = "Error"
		err := cmd.Process.Kill()
		if err != nil {
			glog.Warningf("Couldn't kill %s process %s: %v",
				u.Name, fullCmd, err)
		}
	} else {
		reason = "Completed"
		glog.Infof("Command %v pid %d exited with 0 after %.2fs",
			fullCmd, cmd.Process.Pid, d.Seconds())
	}

	falseval := false
	u.UpdateStatusAttr(&falseval, &falseval)
	u.setTerminatedState(exitCode, reason, startTime)

	if policy == api.RestartPolicyNever ||
		policy == api.RestartPolicyOnFailure && !failure {
		keepGoing = false
	}
	return keepGoing
}

func (u *Unit) setTerminatedState(exitCode int, reason string, startedAt time.Time) {
	t := &api.UnitStateTerminated{
		ExitCode:   int32(exitCode),
		FinishedAt: api.Now(),
		Reason:     reason,
		Message:    u.getTerminationLog(),
		StartedAt:  api.Time{startedAt},
	}
	u.SetState(api.UnitState{Terminated: t}, nil)
}

func (u *Unit) getTerminationLog() string {
	if u.unitConfig.TerminationMessagePolicy != api.TerminationMessageReadFile ||
		u.unitConfig.TerminationMessagePath == "" {
		return ""
	}

	data, err := tailFile(u.unitConfig.TerminationMessagePath, 0, MaxContainerTerminationMessageLength)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		glog.Warningf("Error reading termination message file: %s", err)
	}
	return data
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
	if u.unitConfig.SecurityContext.Capabilities != nil {
		addCaps = u.unitConfig.SecurityContext.Capabilities.Add
		dropCaps = u.unitConfig.SecurityContext.Capabilities.Drop
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

func (u *Unit) applySysctls() error {
	if len(u.unitConfig.Sysctls) == 0 {
		return nil
	}
	for _, sc := range u.unitConfig.Sysctls {
		err := sysctl.Set(sc.Name, sc.Value)
		if err != nil {
			glog.Errorf("Applying sysctl %q=%q: %v", sc.Name, sc.Value, err)
			return err
		}
		glog.Infof("Applied sysctl %q=%q", sc.Name, sc.Value)
	}
	return nil
}

func (u *Unit) setupGpu() error {
	return setupGpu(u.GetRootfs())
}

func (u *Unit) Run(podname string, command []string, workingdir string, policy api.RestartPolicy, mounter mount.Mounter, nser net.NetNamespacer) error {
	unitConfig, err := u.getUnitConfig()
	if err != nil {
		glog.Warningf("getting unit configuration: %v", err)
		return u.doRun(podname, command, workingdir, policy, mounter)
	}
	if api.IsHostNetwork(&unitConfig.PodSecurityContext) {
		glog.Infof("pod %q requested host network mode", podname)
		return u.doRun(podname, command, workingdir, policy, mounter)
	}
	return nser.WithNetNamespace(func() error {
		return u.doRun(podname, command, workingdir, policy, mounter)
	})
}

func (u *Unit) doRun(podname string, command []string, workingdir string, policy api.RestartPolicy, mounter mount.Mounter) error {
	u.SetState(api.UnitState{
		Waiting: &api.UnitStateWaiting{
			Reason: "starting",
		},
	}, nil)

	control, err := cgroups.New(
		cgroups.V1, cgroups.StaticPath("/"+u.Name), &specs.LinuxResources{})
	if err != nil {
		glog.Errorf("Failed to create cgroups control for %q: %v", u.Name, err)
		u.setStateToStartFailure(err)
		return err
	}
	defer control.Delete()
	pid := os.Getpid()
	err = control.Add(cgroups.Process{Pid: pid})
	if err != nil {
		glog.Errorf("Error adding pid %v to cgroups control: %v", pid, err)
		u.setStateToStartFailure(err)
		return err
	}

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
		if err := mounter.MountSpecial(u.Name); err != nil {
			glog.Errorf("mountSpecial(): %v", err)
			u.setStateToStartFailure(err)
			return err
		}
		//  The virtual filesystems (proc, dev, ...) are now mounted into the
		//  rootfs of the unit. If this is a GPU instance, we'll have to do
		//  some extra steps for setting up the unit (mounting in the right
		//  version of support libraries, etc) before calling pivot_root().
		if err := u.setupGpu(); err != nil {
			glog.Errorf("setupGpu(): %v", err)
			mounter.UnmountSpecial(u.Name)
			u.setStateToStartFailure(err)
			return err
		}
		if err := mounter.PivotRoot(rootfs, oldrootfs); err != nil {
			glog.Errorf("PivotRoot() %s %s: %v", rootfs, oldrootfs, err)
			mounter.UnmountSpecial(u.Name)
			u.setStateToStartFailure(err)
			return err
		}
		defer mounter.UnmountSpecial("")
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
		u.statusPath = "/status"
	}

	hostname := makeHostname(podname)
	err = syscall.Sethostname([]byte(hostname))
	if err != nil {
		glog.Errorf("Failed to set hostname to %s: %v", hostname, err)
		u.setStateToStartFailure(err)
		return err
	}

	uid, gid, groups, homedir, err := u.GetUser(&util.OsUserLookup{})
	if err != nil {
		u.setStateToStartFailure(err)
		return err
	}
	if uid != 0 || gid != 0 {
		err = lp.Chown(int(uid), int(gid))
		if err != nil {
			glog.Warningf("Chown %d:%d for pipes: %v", uid, gid, err)
		}
	}

	env := ensureDefaultEnviron(os.Environ(), podname, homedir)
	for _, e := range env {
		items := strings.SplitN(e, "=", 2)
		err = os.Setenv(items[0], items[1])
		if err != nil {
			glog.Errorf("Failed to add default envvar %q: %v", e, err)
			u.setStateToStartFailure(err)
			return err
		}
	}

	err = os.Chmod("/", 0755)
	if err != nil {
		glog.Errorf("Failed to chmod / to 0755: %v", err)
		u.setStateToStartFailure(err)
		return err
	}

	if u.config != nil {
		for vol, _ := range u.config.Volumes {
			err = createDir(vol, int(uid), int(gid))
			if err != nil {
				u.setStateToStartFailure(err)
				return err
			}
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

	err = u.applySysctls()
	if err != nil {
		u.setStateToStartFailure(err)
		return err
	}

	return u.runUnitLoop(command, caplist, uid, gid, groups, unitin, unitout, uniterr, policy)
}

func ensureDefaultEnviron(env []string, podname, homedir string) []string {
	// Make user HOME, HOSTNAME, PATH and TERM are set (same variables Docker
	// ensures are set). See
	// https://docs.docker.com/v17.09/engine/reference/run/#env-environment-variables
	// for more information.
	hostname := makeHostname(podname)
	env = util.AddToEnvList(env, "HOSTNAME", hostname, true)
	env = util.AddToEnvList(env, "TERM", "xterm", false)
	env = util.AddToEnvList(env, "HOME", homedir, false)
	env = util.AddToEnvList(env, "PATH", defaultPath, false)
	return env
}

func makeHostname(podname string) string {
	noNSName := util.GetNameFromString(podname)
	if len(noNSName) > MAX_HOSTNAME_LEN {
		return noNSName[:MAX_HOSTNAME_LEN]
	}
	return noNSName
}
