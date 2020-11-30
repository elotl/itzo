package runtime

import (
	"context"
	"errors"
	"github.com/containers/libpod/v2/pkg/bindings"
	"github.com/containers/libpod/v2/pkg/bindings/containers"
	"github.com/containers/libpod/v2/pkg/bindings/images"
	"github.com/containers/libpod/v2/pkg/bindings/pods"
	"github.com/containers/libpod/v2/pkg/domain/entities"
	"github.com/containers/libpod/v2/pkg/specgen"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/convert"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/golang/glog"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	v1 "k8s.io/api/core/v1"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	PodmanSocketPath string = "unix:/run/podman/podman.sock"
	defaultTimeout   uint   = 30
	RestartPolicyNo = "no"
	// RestartPolicyAlways unconditionally restarts the container.
	RestartPolicyAlways = "always"
	// RestartPolicyOnFailure restarts the container on non-0 exit code,
	// with an optional maximum number of retries.
	RestartPolicyOnFailure = "on-failure"
)

var (
	restartPolicyMap = map[api.RestartPolicy]string{
		api.RestartPolicyAlways: RestartPolicyAlways,
		api.RestartPolicyOnFailure: RestartPolicyOnFailure,
		api.RestartPolicyNever: RestartPolicyNo,
	}
)

type PodmanSandbox struct {
	connText context.Context
}

func (ps *PodmanSandbox) RunPodSandbox(spec *api.PodSpec) error {
	podSpec := specgen.NewPodSpecGenerator()
	podSpec.Name = api.PodName
	hostname := spec.Hostname
	if hostname == "" {
		hostname = api.PodName
	}
	nsMode := specgen.Default
	if api.IsHostNetwork(spec.SecurityContext) {
		nsMode = specgen.Host
	}
	podSpec.NetNS = specgen.Namespace{
		NSMode: nsMode,
		Value:  "",
	}
	podSpec.Hostname = hostname
	// those two are important as we deploy and mount resolv.conf and /etc/hosts
	podSpec.NoManageResolvConf = true
	podSpec.NoManageHosts = true
	portMappings := make([]specgen.PortMapping, 0)
	for _, unit := range spec.Units {
		for _, port := range unit.Ports {
			portMappings = append(portMappings, specgen.PortMapping{
				ContainerPort: uint16(port.ContainerPort),
				HostPort:      uint16(port.HostPort),
				Protocol:      string(port.Protocol),
			})
		}
	}
	podSpec.PortMappings = portMappings
	_, err := pods.CreatePodFromSpec(ps.connText, podSpec)
	if err != nil {
		glog.Errorf("error creating pod from spec: %v", err)
	}
	return err
}

func (ps *PodmanSandbox) StopPodSandbox(spec *api.PodSpec) error {
	report, err := pods.Stop(ps.connText, api.PodName, nil)
	if report != nil && len(report.Errs) > 0 {
		return errors.New("TODO")
	}
	return err
}
func (ps *PodmanSandbox) RemovePodSandbox(spec *api.PodSpec) error {
	report, err := pods.Remove(ps.connText, api.PodName, nil)
	if report != nil && report.Err != nil {
		return report.Err
	}
	return err
}
func (ps *PodmanSandbox) PodSandboxStatus() error {
	_, err := pods.Inspect(ps.connText, api.PodName)
	if err != nil {
		return err
	}

	return nil
}

type PodmanImageService struct {
	connText context.Context
}

func (p *PodmanImageService) ListImages() {
	return
}

func (p *PodmanImageService) ImageStatus(rootdir, image string) error {
	return nil
}

func (p *PodmanImageService) PullImage(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials) error {
	//exists, err := images.Exists(p.connText, image)
	//if exists {
	//	return nil
	//}
	//if err != nil {
	//	glog.Errorf("error checking if image %s already exists: %v", image, err)
	//}
	//_, _, err = util.ParseImageSpec(image)
	//if err != nil {
	//	return err
	//}
	// TODO handle registry creds
	opts := entities.ImagePullOptions{}
	//username, password := util.GetRepoCreds(server, registryCredentials)
	////opts := entities.ImagePullOptions{
	////	Username: username,
	////	Password: password,
	////}
	glog.Infof("trying to pull image: %s for container: %s", image, name)
	_, err := images.Pull(p.connText, image, opts)
	return err
}

func (p *PodmanImageService) RemoveImage(rootdir, image string) error {
	_, err := images.Remove(p.connText, image, false)
	return err
}

type PodmanContainerService struct {
	rootdir   string
	imgPuller PodmanImageService
}

func NewPodmanContainerService(ctx context.Context, rootdir string) *PodmanContainerService {
	return &PodmanContainerService{rootdir: rootdir, imgPuller: PodmanImageService{connText: ctx}}
}

func (pcs *PodmanContainerService) CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials) (*api.UnitStatus, error) {
	container := convert.UnitToK8sContainer(unit)
	var k8sVolumes []v1.Volume
	for _, vol := range spec.Volumes {
		k8sVolumes = append(k8sVolumes, convert.VolumeToK8sVolume(vol))
	}
	// TODO: ignore this failure for now, it seems that there's response serialization bug on podman site

	_ = pcs.imgPuller.PullImage(pcs.rootdir, unit.Name, unit.Image, registryCredentials)
	//if err != nil {
	//	glog.Errorf("pulling image %s for container %s failed with: %v", unit.Image, unit.Name, err)
	//	return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "Pulling image failed"), err
	//}
	containerSpec := specgen.NewSpecGenerator(container.Image, false)
	containerSpec.Name = convert.UnitNameToContainerName(unit.Name)
	containerSpec.Pod = api.PodName
	containerSpec.Terminal = true
	containerSpec.Command = unit.Command
	containerSpec.RestartPolicy = restartPolicyMap[spec.RestartPolicy]
	containerSpec.Env = make(map[string]string)
	containerSpec.Mounts = make([]runtimespec.Mount, 0)

	// TODO - examine if it's needed
	for _, env := range container.Env {
		if env.ValueFrom == nil {
			containerSpec.Env[env.Name] = env.Value
		}
	}
	for _, mount := range container.VolumeMounts {
		var volume v1.Volume
		for _, vol := range spec.Volumes {
			if vol.Name == mount.Name {
				volume = convert.VolumeToK8sVolume(vol)
				break
			}
		}
		path := volume.HostPath.Path
		if mount.SubPath != "" {
			path = filepath.Join(path, mount.SubPath)
		}
		containerSpec.Mounts = append(containerSpec.Mounts, runtimespec.Mount{
			Destination: mount.MountPath,
			Type:        "bind",
			Source:      path,
			Options:     nil,
		})
	}
	_, err := containers.CreateWithSpec(pcs.imgPuller.connText, containerSpec)
	if err != nil {
		glog.Errorf("error from contrainer.CreateWithSpec: %v", err)
		//return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "podman failed to start container"), err
	}
	glog.Infof("podman started unit %s as container %s with %s image", unit.Name, container.Name, unit.Image)
	return api.MakeStillCreatingStatus(unit.Name, unit.Image, "Container created"), nil
}

func (pcs *PodmanContainerService) StartContainer(unit api.Unit, spec *api.PodSpec, podName string) (*api.UnitStatus, error) {
	podmanContainerName := convert.UnitNameToContainerName(unit.Name)
	err := containers.Start(pcs.imgPuller.connText, podmanContainerName, nil)
	if err != nil {
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "runtime cannot start container"), err
	}
	return nil, nil
}

func (pcs *PodmanContainerService) RemoveContainer(unit *api.Unit) error {
	containerName := convert.UnitNameToContainerName(unit.Name)
	err := containers.Stop(pcs.imgPuller.connText, containerName, nil)
	if err != nil {
		return err
	}
	force := true
	deleteVolumes := false
	err = containers.Remove(pcs.imgPuller.connText, containerName, &force, &deleteVolumes)
	return err
}

func (pcs *PodmanContainerService) ListContainers() error { return nil }
func (pcs *PodmanContainerService) ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error) {
	containerName := convert.UnitNameToContainerName(unitName)
	ctrData, err := containers.Inspect(pcs.imgPuller.connText, containerName, nil)
	if err != nil {
		// TODO - distinguish situations:
		// A - container died (for whatever reason)
		// B - container is stooped by pod controller and will be recreated with newly image (because it got updated by user)
		// if we return failed status kip marks pod as failed and reschedules it on new instance
		//errorMsg := fmt.Sprintf("cannot get container state from podman for unit: %s\n%v \nctrData: %v", containerName, err, ctrData)
		return api.MakeStillCreatingStatus(unitName, ctrData.Image, "Container stopped"), nil
		//return api.MakeFailedUpdateStatus(unitName, ctrData.Image, "Container runtime failure"), errors.New(errorMsg)

	}
	state, ready := convert.ContainerStateToUnit(*ctrData)
	status := &api.UnitStatus{
		Name:         unitName,
		State:        state,
		RestartCount: 0,
		Image:        unitImage,
		Ready:        ready,
	}
	return status, nil
}

type PodmanRuntime struct {
	PodmanSandbox
	PodmanContainerService
}

func (p *PodmanRuntime) Status() {
	return
}

func (p *PodmanRuntime) GetLogBuffer(unitName string) (*logbuf.LogBuffer, error) {
	return nil, nil
}

func (p *PodmanRuntime) ReadLogBuffer(unit string, n int) ([]logbuf.LogEntry, error) {
	containerName := convert.UnitNameToContainerName(unit)
	yes := true
	tail := strconv.Itoa(n)
	out := make(chan string)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	opts := containers.LogOptions{
		Stderr:     &yes,
		Stdout:     &yes,
		Tail:       &tail,
		Timestamps: &yes,
	}
	var logs []logbuf.LogEntry
	go func(wg *sync.WaitGroup) {
		err := containers.Logs(p.connText, containerName, opts, out, out)
		if err != nil {
			glog.Errorf("cannot get logs for container %s : %v", containerName, err)
		}
		wg.Done()
		close(out)
	}(wg)
	for msg := range out {
		logLine := strings.Split(msg, " ")
		logs = append(logs, logbuf.LogEntry{
			Timestamp: logLine[0],
			Source:    logbuf.StdoutLogSource,
			Line:      strings.Join(logLine[1:], " "),
		})
	}
	wg.Wait()
	return logs, nil
}

func (p *PodmanRuntime) UnitRunning(unitName string) bool {
	unitStatus, err := p.ContainerStatus(unitName, "")
	if err != nil {
		glog.Error(err)
		return false
	}
	if unitStatus.State.Running == nil {
		return false
	}
	return true
}

func (p *PodmanRuntime) GetPid(unitName string) (int, bool) {
	return 0, false
}

func (p *PodmanRuntime) SetPodNetwork(netNS, podIP string) {
	return
}

func NewPodmanRuntime(rootdir string) (*PodmanRuntime, error) {
	connText, err := GetPodmanConnection()
	if err != nil {
		return nil, err
	}
	containerService := NewPodmanContainerService(connText, rootdir)
	return &PodmanRuntime{
		PodmanSandbox:          PodmanSandbox{connText: connText},
		PodmanContainerService: *containerService,
	}, nil
}

func GetPodmanConnection() (context.Context, error) {
	// Connect to Podman socket
	connText, err := bindings.NewConnection(context.Background(), PodmanSocketPath)
	return connText, err
}
