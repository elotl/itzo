package runtime

import (
	"bufio"
	"fmt"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	itzounit "github.com/elotl/itzo/pkg/unit"
	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"io"
	"strings"
)

type Puller interface {
	PullImage(rootdir, name, image, server, username, password string) error
}

type Mounter interface {
	CreateMount(*api.Volume) error
	DeleteMount(*api.Volume) error
	AttachMount(unitname, src, dst string) error
	DetachMount(unitname, dst string) error
}

// Too bad there isn't a word for a creator AND destroyer
// Coulda gone with Shiva(er) but that's a bit imprecise...
type UnitRunner interface {
	StartUnit(string, string, string, string, string, []string, []string, []string, api.RestartPolicy) error
	StopUnit(string) error
	RemoveUnit(string) error
	UnitRunning(string) bool
	GetLogBuffer(unitName string) (*logbuf.LogBuffer, error)
	ReadLogBuffer(unitName string, n int) ([]logbuf.LogEntry, error)
	GetPid(string) (int, bool)
}

type ImagePuller struct {
}

func (ip *ImagePuller) PullImage(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) error {
	server, _, err := util.ParseImageSpec(image)
	if err != nil {
		msg := fmt.Sprintf("Bad image spec for unit %s: %v", name, err)
		return errors.Wrapf(err, msg)
	}
	username, password := util.GetRepoCreds(server, registryCredentials)

	if server == "docker.io" {
		// K8s and Helm might set this for images, but the actual official
		// registry is registry-1.docker.io.
		server = "registry-1.docker.io"
	}
	if server != "" && !strings.HasPrefix(server, "http") {
		server = "https://" + server
	}
	glog.Infof("Creating new unit '%s' in %s\n", name, rootdir)
	u, err := itzounit.OpenUnit(rootdir, name)
	if err != nil {
		return errors.Wrapf(err, "opening unit %s for package deploy", name)
	}
	u.SetUnitConfigOverlayfs(useOverlayfs)
	err = u.PullAndExtractImage(image, server, username, password)
	if err != nil {
		return errors.Wrapf(err, "pulling image %s", image)
	}
	return nil
}

type ItzoRuntime struct {
	rootdir   string
	UnitMgr   UnitRunner
	MountCtl  Mounter
	ImgPuller ImageService
	netNS     string
	podIP     string
}

func NewItzoRuntime(rootdir string, unitMgr UnitRunner, mounter Mounter, imgPuller ImageService) *ItzoRuntime {
	return &ItzoRuntime{
		rootdir:   rootdir,
		UnitMgr:   unitMgr,
		MountCtl:  mounter,
		ImgPuller: imgPuller,
	}
}

func (i *ItzoRuntime) RunPodSandbox(spec *api.PodSpec) error {
	glog.Info("status units are nil, trying to create pod from scratch")
	for _, volume := range spec.Volumes {
		err := i.MountCtl.CreateMount(&volume)
		if err != nil {
			glog.Errorf("Error creating volume: %s, %v", volume.Name, err)
			return err
		}
	}
	spec.Phase = api.PodDispatching
	return nil
}

func (i *ItzoRuntime) StopPodSandbox(spec *api.PodSpec) error {
	return nil
}

func (i *ItzoRuntime) RemovePodSandbox(spec *api.PodSpec) error {
	for _, unit := range append(spec.InitUnits, spec.Units...) {
		err := i.RemoveContainer(&unit)
		if err != nil {
			glog.Errorf("error while destroying unit: %s: %v", unit.Name, err)
			return err
		}
	}
	for _, volume := range spec.Volumes {
		err := i.MountCtl.DeleteMount(&volume)
		if err != nil {
			glog.Errorf("Error removing volume %s: %v", volume.Name, err)
			return err
		}
	}
	return nil
}

func (i *ItzoRuntime) CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) (*api.UnitStatus, error) {
	// pull image
	err := i.ImgPuller.PullImage(i.rootdir, unit.Name, unit.Image, registryCredentials, useOverlayfs)
	if err != nil {
		msg := fmt.Sprintf("Bad image spec for unit %s: %v", unit.Name, err)
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, msg), err
	}
	err = i.saveUnitConfig(&unit, spec.SecurityContext)
	if err != nil {
		msg := fmt.Sprintf("Error saving unit %s configuration: %v",
			unit.Name, err)
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, msg), err
	}
	// attach mounts
	for _, volMount := range unit.VolumeMounts {
		err := i.MountCtl.AttachMount(
			unit.Name, volMount.Name, volMount.MountPath)
		if err != nil {
			msg := fmt.Sprintf("Error attaching mount %s to unit %s: %v",
				volMount.Name, unit.Name, err)
			return api.MakeFailedUpdateStatus(unit.Name, unit.Image, msg), err
		}
	}
	return nil, nil
}

func (i *ItzoRuntime) StartContainer(unit api.Unit, podSpec *api.PodSpec, podName string) (*api.UnitStatus, error) {
	err := i.UnitMgr.StartUnit(
		podName,
		podSpec.Hostname,
		unit.Name,
		unit.WorkingDir,
		i.netNS,
		unit.Command,
		unit.Args,
		makeAppEnv(&unit),
		podSpec.RestartPolicy)
	if err != nil {
		msg := fmt.Sprintf("Error starting unit %s: %v",
			unit.Name, err)
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, msg), err
	}
	return nil, nil
}

func (i *ItzoRuntime) RemoveContainer(unit *api.Unit) error {
	unitName := unit.Name
	glog.Infoln("Stopping unit", unitName)
	//
	// There's a few things here that need to happen in order:
	//   * Stop the unit (kill its main process).
	//   * Detach all its mounts.
	//   * Remove its files/directories.
	//
	err := i.UnitMgr.StopUnit(unitName)
	if err != nil {
		glog.Errorf("Error stopping unit %s: %v; trying to continue",
			unitName, err)
	}
	for _, volMount := range unit.VolumeMounts {
		err = i.MountCtl.DetachMount(unitName, volMount.MountPath)
		if err != nil {
			glog.Errorf(
				"Error detaching mount %s from %s: %v; trying to continue",
				volMount.Name, unitName, err)
		}
	}
	err = i.UnitMgr.RemoveUnit(unitName)
	if err != nil {
		glog.Errorf("Error removing unit %s; trying to continue",
			unitName)
	}
	return err
}

func (i *ItzoRuntime) ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error) {
	if i.UnitMgr.UnitRunning(unitName) {
		return &api.UnitStatus{
			Name: unitName,
			State: api.UnitState{
				Running: &api.UnitStateRunning{},
			},
			RestartCount: 0,
			Image:        unitImage,
			Ready:        true,
		}, nil
	}
	if !itzounit.IsUnitExist(i.rootdir, unitName) {
		reason := "PodInitializing"
		return api.MakeStillCreatingStatus(
			unitName, unitImage, reason), nil
	}
	openedUnit, err := itzounit.OpenUnit(i.rootdir, unitName)
	if err != nil {
		reason := "PodInitializing"
		return api.MakeStillCreatingStatus(
			unitName, unitImage, reason), err
	}
	us, err := openedUnit.GetStatus()
	if err != nil {
		reason := "PodInitializing"
		return api.MakeStillCreatingStatus(
			unitName, unitImage, reason), nil
	}
	return us, nil
}

func (i *ItzoRuntime) GetLogBuffer(options LogOptions) (*logbuf.LogBuffer, error) {
	return i.UnitMgr.GetLogBuffer(options.UnitName)
}

func (i *ItzoRuntime) UnitRunning(unitName string) bool {
	return i.UnitMgr.UnitRunning(unitName)
}

func (i *ItzoRuntime) GetPid(unitName string) (int, bool) {
	return i.UnitMgr.GetPid(unitName)
}

func (i *ItzoRuntime) SetPodNetwork(netNS, podIP string)  {
	i.podIP = podIP
	i.netNS = netNS
}

func (i *ItzoRuntime) Exec(params api.ExecParams, stdOutWriter, stdErrWriter io.WriteCloser, reader *bufio.Reader) error {
	//unitName, err := i.GetUnitName(params.UnitName)
	//if err != nil {
	//	glog.Errorf("Getting unit %s: %v", params.UnitName, err)
	//	return err
	//}
	//
	//command := params.Command
	//
	//var env []string
	//
	//// allow us to skip entering namespace for testing
	//if !params.SkipNSEnter {
	//	unit, err := unit.OpenUnit(s.installRootdir, unitName)
	//	if err != nil {
	//		errmsg := fmt.Errorf("error opening unit %s for exec: %v", unitName, err)
	//		glog.Errorf("%v", errmsg)
	//		return errmsg
	//	}
	//	userLookup, err := util.NewPasswdUserLookup(unit.GetRootfs())
	//	if err != nil {
	//		errmsg := fmt.Errorf("error creating user lookup in %s for exec: %v", unitName, err)
	//		glog.Errorf("%v", errmsg)
	//		return errmsg
	//	}
	//	uid, gid, _, homedir, err := unit.GetUser(userLookup)
	//	if err != nil {
	//		errmsg := fmt.Errorf("error getting unit %s user for exec: %v", unitName, err)
	//		glog.Errorf("%v", errmsg)
	//		return errmsg
	//	}
	//	pid, exists := s.podController.GetPid(unitName)
	//	if !exists {
	//		glog.Errorf("Error getting pid for unit %s", unitName)
	//		return fmt.Errorf("Could not find running process for unit named %s\n", unitName)
	//	}
	//	proc, err := procfs.NewProcess(pid, false)
	//	if err != nil {
	//		glog.Errorf("Error getting process for unit %s", unitName)
	//		return fmt.Errorf("Could not find process %d for unit named %s\n", pid, unitName)
	//	}
	//	for k, v := range proc.Environ {
	//		env = append(env, fmt.Sprintf("%s=%s", k, v))
	//	}
	//	env = helper.EnsureDefaultEnviron(env, params.PodName, homedir)
	//	nsenterCmd := []string{
	//		"/usr/bin/nsenter",
	//		"-t",
	//		strconv.Itoa(pid),
	//		"-p",
	//		"-u",
	//		"-m",
	//		"-n",
	//	}
	//	if uid != 0 || gid != 0 {
	//		userSpec := []string{
	//			"-S",
	//			fmt.Sprintf("%d", uid),
	//			"-G",
	//			fmt.Sprintf("%d", gid),
	//		}
	//		nsenterCmd = append(nsenterCmd, userSpec...)
	//	}
	//	command = append(nsenterCmd, command...)
	//}
	//
	//glog.Infof("Exec command: %s", command[0])
	//cmd := exec.Command(command[0], command[1:]...)
	//cmd.Env = env
	//if params.TTY {
	//	err = s.runExecTTY(ws, cmd, params.Interactive)
	//} else {
	//	err = s.runExecCmd(ws, cmd, params.Interactive)
	//}
	//if err != nil {
	//	glog.Errorf("Error running exec command %s: %v", command[0], err)
	//	return err
	//}
	return nil
}

func (i *ItzoRuntime) saveUnitConfig(unit *api.Unit, podSecurityContext *api.PodSecurityContext) error {
	unitConfig := itzounit.UnitConfig{
		StartupProbe:             util.TranslateProbePorts(unit, unit.StartupProbe),
		ReadinessProbe:           util.TranslateProbePorts(unit, unit.ReadinessProbe),
		LivenessProbe:            util.TranslateProbePorts(unit, unit.LivenessProbe),
		TerminationMessagePolicy: unit.TerminationMessagePolicy,
		TerminationMessagePath:   unit.TerminationMessagePath,
		PodIP:                    i.podIP,
	}
	if podSecurityContext != nil {
		unitConfig.PodSecurityContext = *podSecurityContext
	}
	if unit.SecurityContext != nil {
		unitConfig.SecurityContext = *unit.SecurityContext
	}
	u, err := itzounit.OpenUnit(i.rootdir, unit.Name)
	if err != nil {
		return fmt.Errorf("opening unit %q for saving unit configuration: %v",
			unit.Name, err)
	}
	err = u.SaveUnitConfig(unitConfig)
	if err != nil {
		return fmt.Errorf("saving unit %q configuration: %v",
			unit.Name, err)
	}
	return nil
}

func makeAppEnv(unit *api.Unit) []string {
	var e []string
	for _, ev := range unit.Env {
		e = append(e, fmt.Sprintf("%s=%s", ev.Name, ev.Value))
	}
	return e
}
