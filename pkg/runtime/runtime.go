package runtime

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/metrics"
)

type PodSandbox interface {
	RunPodSandbox(spec *api.PodSpec) error
	StopPodSandbox(spec *api.PodSpec) error
	RemovePodSandbox(spec *api.PodSpec) error
}

type ContainerService interface {
	CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) (*api.UnitStatus, error)
	StartContainer(unit api.Unit, spec *api.PodSpec, podName string) (*api.UnitStatus, error)
	RemoveContainer(unit *api.Unit) error
	ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error)
	//ExecSync()
	//Exec()
	//Attach()
	//PortForward()
	// TODO:: those below are needed for server handlers, we should think about ways to remove them from this interface
	GetLogBuffer(unitName string) (*logbuf.LogBuffer, error)
	ReadLogBuffer(unitName string, n int) ([]logbuf.LogEntry, error)
	UnitRunning(unitName string) bool
	GetPid(unitName string) (int, bool)
	SetPodNetwork(netNS, podIP string)
}

// This is heavily based on Kubernetes CRI.
// Methods we don't really need are commented out.
// Having it that similar to CRI opens a door for using it in the future.
type RuntimeService interface {
	PodSandbox
	ContainerService
	metrics.MetricsProvider
}

type ImageService interface {
	PullImage(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) error
}
