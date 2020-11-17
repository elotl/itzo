package runtime

import (
	"context"
	"errors"
	"fmt"
	"github.com/containers/libpod/v2/pkg/bindings"
	"github.com/containers/libpod/v2/pkg/bindings/containers"
	"github.com/containers/libpod/v2/pkg/bindings/images"
	"github.com/containers/libpod/v2/pkg/bindings/pods"
	"github.com/containers/libpod/v2/pkg/domain/entities"
	"github.com/containers/libpod/v2/pkg/specgen"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/convert"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	v1 "k8s.io/api/core/v1"
)

const (
	podName          string = "mypod"
	PodmanSocketPath string = "unix:/run/podman/podman.sock"
	defaultTimeout   uint    = 30
)

type PodmanSandbox struct {
	connText context.Context
}

func (ps *PodmanSandbox) RunPodSandbox(spec *api.PodSpec) error {
	podSpec := specgen.NewPodSpecGenerator()
	podSpec.Name = podName
	hostname := spec.Hostname
	if hostname == "" {
		hostname = podName
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
	_, err := pods.CreatePodFromSpec(ps.connText, podSpec)
	return err
}

func (ps *PodmanSandbox) StopPodSandbox() error {
	report, err := pods.Stop(ps.connText, podName, nil)
	if report != nil && len(report.Errs) > 0 {
		return errors.New("TODO")
	}
	return err
}
func (ps *PodmanSandbox) RemovePodSandbox() error {
	report, err := pods.Remove(ps.connText, podName, nil)
	if report != nil && report.Err != nil {
		return report.Err
	}
	return err
}
func (ps *PodmanSandbox) PodSandboxStatus() error {
	_, err := pods.Inspect(ps.connText, podName)
	if err != nil {
		return err
	}

	return nil
}

type PodmanImageService struct {
	connText context.Context
}

func (p *PodmanImageService) ListImages() {
	panic("implement me")
}

func (p *PodmanImageService) ImageStatus(rootdir, image string) error {
	panic("implement me")
}

func (p *PodmanImageService) PullImage(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials) error {
	server, _, err := util.ParseImageSpec(image)
	if err != nil {
		return err
	}
	username, password := util.GetRepoCreds(server, registryCredentials)
	opts := entities.ImagePullOptions{
		Username: username,
		Password: password,
	}
	_, err = images.Pull(p.connText, image, opts)
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
	err := pcs.imgPuller.PullImage(pcs.rootdir, unit.Name, unit.Image, registryCredentials)
	if err != nil {
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "Pulling image failed"), err
	}
	containerSpec := specgen.NewSpecGenerator(container.Image, true)
	containerSpec.Pod = api.PodmanPodName
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
		containerSpec.Mounts = append(containerSpec.Mounts, runtimespec.Mount{
			Destination: mount.MountPath,
			Type:        "bind",
			Source:      volume.HostPath.Path,
			Options:     nil,
		})
	}
	//containerSpec.Mounts
	//report, err := containers.CreateWithSpec(pcs.connText, containerSpec)
	return nil, nil
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

func (pcs *PodmanContainerService) ListContainers() error                { return nil }
func (pcs *PodmanContainerService) ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error) {
	containerName := convert.UnitNameToContainerName(unitName)
	ctrData, err := containers.Inspect(pcs.imgPuller.connText, containerName, nil)
	if err != nil {
		errorMsg := fmt.Sprintf("cannot get container state from podman for unit: %s\n%v \nctrData: %v", containerName, err, ctrData)
		return api.MakeFailedUpdateStatus(unitName, ctrData.Image, "Container runtime failure"), errors.New(errorMsg)

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
	imgPuller PodmanImageService
}

func (p *PodmanRuntime) getCtx() context.Context {
	return p.connText
}

func (p *PodmanRuntime) StopPodSandbox(spec *api.PodSpec) error {
	_, err := pods.Stop(p.connText, podName, nil)
	return err
}

func (p *PodmanRuntime) RemovePodSandbox(spec *api.PodSpec) error {
	panic("implement me")
}

func (p *PodmanRuntime) Status() {
	panic("implement me")
}

func (p *PodmanRuntime) GetLogBuffer(unitName string) (*logbuf.LogBuffer, error) {
	panic("implement me")
}

func (p *PodmanRuntime) ReadLogBuffer(unit string, n int) ([]logbuf.LogEntry, error) {
	panic("implent me")
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
	panic("not implemented")
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
