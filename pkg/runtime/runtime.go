package runtime

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
)

type PodSandbox interface {
	RunPodSandbox(spec *api.PodSpec) error
	StopPodSandbox(spec *api.PodSpec) error
	RemovePodSandbox(spec *api.PodSpec) error
	PodSandboxStatus() error
	//ListPodSandbox() error
}

type ContainerService interface {
	CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials) error
	StartContainer(unit api.Unit, spec *api.PodSpec, podName string) error
	RemoveContainer(unit *api.Unit) error
	ListContainers() error
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
	//Version()
	PodSandbox
	ContainerService
	//UpdateRuntimeConfig()
	Status()

}

type ImageService interface {
	ListImages()
	ImageStatus(rootdir, image string)
	PullImage(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials) error
	RemoveImage(rootdir, image string)
}
