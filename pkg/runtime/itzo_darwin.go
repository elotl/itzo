package runtime

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
)

type ImagePuller struct {}

func (i ImagePuller) PullImage(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) error {
	return nil
}

type ItzoRuntime struct {}

func (i ItzoRuntime) RunPodSandbox(spec *api.PodSpec) error {
	panic("implement me")
}

func (i ItzoRuntime) StopPodSandbox(spec *api.PodSpec) error {
	panic("implement me")
}

func (i ItzoRuntime) RemovePodSandbox(spec *api.PodSpec) error {
	panic("implement me")
}

func (i ItzoRuntime) CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) (*api.UnitStatus, error) {
	panic("implement me")
}

func (i ItzoRuntime) StartContainer(unit api.Unit, spec *api.PodSpec, podName string) (*api.UnitStatus, error) {
	panic("implement me")
}

func (i ItzoRuntime) RemoveContainer(unit *api.Unit) error {
	panic("implement me")
}

func (i ItzoRuntime) ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error) {
	panic("implement me")
}

func (i ItzoRuntime) GetLogBuffer(options LogOptions) (*logbuf.LogBuffer, error) {
	panic("implement me")
}

func (i ItzoRuntime) UnitRunning(unitName string) bool {
	panic("implement me")
}

func (i ItzoRuntime) GetPid(unitName string) (int, bool) {
	panic("implement me")
}

func (i ItzoRuntime) SetPodNetwork(netNS, podIP string) {
	panic("implement me")
}

func (i ItzoRuntime) ReadSystemMetrics(s string) api.ResourceMetrics {
	panic("implement me")
}

func (i ItzoRuntime) ReadUnitMetrics(s string) api.ResourceMetrics {
	panic("implement me")
}

func NewItzoRuntime(rootdir string, unitMgr UnitRunner, mounter Mounter, imgPuller ImageService) *ItzoRuntime {
	return &ItzoRuntime{}
}

