package mac

import (
	"fmt"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/metrics"
	"github.com/elotl/itzo/pkg/runtime"
	"strings"
	"sync"
)

const (
	vmImagePrefix = "mac-anka-img"
)

type MacRuntime struct {
	metrics.AnkaMetricsProvider
	registryClient RegistryClient
	cliClient      *AnkaCLI
	UnitsVMIDs     *sync.Map
}

func NewMacRuntime(registryClient RegistryClient) *MacRuntime {
	return &MacRuntime{
		registryClient: registryClient,
		cliClient:      NewAnkaCLI(CMDExecWrapper),
		UnitsVMIDs:     new(sync.Map),
	}
}

func (m *MacRuntime) RunPodSandbox(spec *api.PodSpec) error {
	return m.cliClient.EnsureAnkaBin()
}

func (m *MacRuntime) StopPodSandbox(spec *api.PodSpec) error {
	return nil
}

func (m *MacRuntime) RemovePodSandbox(spec *api.PodSpec) error {
	return nil
}

func (m *MacRuntime) CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) (*api.UnitStatus, error) {
	// check if image exists in registry
	imageID, err := getVMImageIDFromUnitImage(unit)
	if err != nil {
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "InvalidImgFormat"), err
	}
	err = m.registryClient.GetVMTemplate(imageID)
	if err != nil {
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "VMTemplateNotFound"), err
	}
	err = m.cliClient.PullImage(imageID)
	if err != nil {
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "VMTemplatePullFailed"), err
	}
	return api.MakeStillCreatingStatus(unit.Name, unit.Image, "VMTemplatePulled"), nil
}

func (m *MacRuntime) StartContainer(unit api.Unit, spec *api.PodSpec, podName string) (*api.UnitStatus, error) {
	vmIDInterface, ok := m.UnitsVMIDs.Load(unit.Name)
	if !ok {
		return nil, fmt.Errorf("cannot find vm id for unit %s", unit.Name)
	}
	vmID := vmIDInterface.(string)
	unitStatus, err := m.cliClient.Start(vmID)
	if err != nil {
		return nil, err
	}
	if unitStatus.Status != AnkaStatusOK {
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "VMStartFailed"), fmt.Errorf("cannot start vm: %s", unitStatus.Message)
	}
	started := true
	return &api.UnitStatus{
		Name: unit.Name,
		State: api.UnitState{
			Running: &api.UnitStateRunning{StartedAt: api.Now()},
		},
		Image:   unit.Image,
		Ready:   true,
		Started: &started,
	}, nil
}

func getVMImageIDFromUnitImage(unit api.Unit) (string, error) {
	// Let's set a convention that if container image has prefix mac-anka-img:
	// then we'll treat suffix as anka VM Image ID.
	// e.g. mac-anka-img:<vm-id>
	vmImgName := strings.Split(unit.Image, ":")
	if len(vmImgName) < 2 {
		return "", fmt.Errorf("image has to be in format %s:vm-id", vmImagePrefix)
	}
	if vmImgName[0] != vmImagePrefix {
		return "", fmt.Errorf("image has to be in format %s:vm-id", vmImagePrefix)
	}
	return vmImgName[1], nil
}

func (m *MacRuntime) RemoveContainer(unit *api.Unit) error {
	vmIDInterface, ok := m.UnitsVMIDs.Load(unit.Name)
	if !ok {
		return fmt.Errorf("cannot find vm id for unit %s", unit.Name)
	}
	vmID := vmIDInterface.(string)
	stopResp, err := m.cliClient.Stop(vmID)
	if err != nil {
		return err
	}
	if stopResp.Status != AnkaStatusOK {
		return fmt.Errorf("cannot stop vm with id %s", vmID)
	}
	m.UnitsVMIDs.Delete(unit.Name)
	return nil
}

func (m *MacRuntime) ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error) {
	// TODO
	vmIDinterface, ok := m.UnitsVMIDs.Load(unitName)
	if !ok {
		return nil, fmt.Errorf("cannot find vm id for unit %s", unitName)
	}
	vmID := vmIDinterface.(string)
	showOutput, err := m.cliClient.Show(vmID)
	if err != nil {
		return nil, err
	}
	switch showOutput.Body.Status {
	case "stopped":
		// vm is stopped
		return api.MakeFailedUpdateStatus(unitName, unitImage, "VMStopped"), nil
	case "suspended":
		// vm is suspended
		return api.MakeFailedUpdateStatus(unitName, unitImage, "VMSuspended"), nil
	case "running":
		return &api.UnitStatus{
			Name:  unitName,
			State: api.UnitState{Running: &api.UnitStateRunning{}},
			Image: unitImage,
		}, nil
	case "failed":
		return api.MakeFailedUpdateStatus(unitName, unitImage, "VMFailed"), nil

	}
	return nil, nil
}

func (m *MacRuntime) GetLogBuffer(options runtime.LogOptions) (*logbuf.LogBuffer, error) {
	return nil, nil
}

func (m *MacRuntime) UnitRunning(unitName string) bool {
	return true
}

func (m *MacRuntime) GetPid(unitName string) (int, bool) {
	return 0, false
}

func (m *MacRuntime) SetPodNetwork(netNS, podIP string) {
	return
}
