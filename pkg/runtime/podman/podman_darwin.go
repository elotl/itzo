package podman

import (
	"context"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/runtime"
)

type NoOpPodmanRuntime struct {}

func (n NoOpPodmanRuntime) GetLogBuffer(options runtime.LogOptions) (*logbuf.LogBuffer, error) {
	panic("implement me")
}

func (n NoOpPodmanRuntime) ReadSystemMetrics(s string) api.ResourceMetrics {
	panic("implement me")
}

func (n NoOpPodmanRuntime) ReadUnitMetrics(s string) api.ResourceMetrics {
	panic("implement me")
}

func (n NoOpPodmanRuntime) RunPodSandbox(spec *api.PodSpec) error {
	return nil
}

func (n NoOpPodmanRuntime) StopPodSandbox(spec *api.PodSpec) error {
	return nil
}

func (n NoOpPodmanRuntime) RemovePodSandbox(spec *api.PodSpec) error {
	return nil
}

func (n NoOpPodmanRuntime) CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) (*api.UnitStatus, error) {
	return nil, nil
}

func (n NoOpPodmanRuntime) StartContainer(unit api.Unit, spec *api.PodSpec, podName string) (*api.UnitStatus, error) {
	return nil, nil
}

func (n NoOpPodmanRuntime) RemoveContainer(unit *api.Unit) error {
	return nil
}

func (n NoOpPodmanRuntime) ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error) {
	return nil, nil
}

func (n NoOpPodmanRuntime) UnitRunning(unitName string) bool {
	return true
}

func (n NoOpPodmanRuntime) GetPid(unitName string) (int, bool) {
	return 0, false
}

func (n NoOpPodmanRuntime) SetPodNetwork(netNS, podIP string) {
	return
}

func NewPodmanRuntime(rootdir string) (*NoOpPodmanRuntime, error) {
	return &NoOpPodmanRuntime{}, nil
}

func GetPodmanConnection() (context.Context, error) {
	return context.TODO(), nil
}
