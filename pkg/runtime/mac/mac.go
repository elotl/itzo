package mac

import (
	"fmt"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/metrics"
	"github.com/elotl/itzo/pkg/runtime"
	"github.com/golang/glog"
	"sync"
	"time"
)

const (
	vmImagePrefix = "mac-anka-img"
)

var (
	datetimeAnkaLayout = "2006-01-02T15:04:05.000000Z"
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
	// ensure anka bin
	err := m.cliClient.EnsureAnkaBin()
	if err != nil {
		return err
	}
	// Not needed anymore since we now have license that doesn't require activating
	//err = m.cliClient.ActivateLicense("") // TODO
	//if err != nil {
	//	return err
	//}
	//err = m.cliClient.ValidateLicense()
	//if err != nil {
	//	return err
	//}
	return nil
}

func (m *MacRuntime) StopPodSandbox(spec *api.PodSpec) error {
	return nil
}

func (m *MacRuntime) RemovePodSandbox(spec *api.PodSpec) error {
	return nil
}

func (m *MacRuntime) CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) (*api.UnitStatus, error) {
	// check if vm already exist
	// if it doesn't, check if vm template exists in registry
	vmId, _ := parseImageUrl(unit.Image)
	_, err := m.cliClient.Show(vmId)
	if err == nil {
		m.UnitsVMIDs.Store(unit.Name, vmId)
		return api.MakeStillCreatingStatus(unit.Name, unit.Image, "VMCreated"), nil
	}
	vmId, err = m.registryClient.GetVMTemplate(unit.Image)
	if err != nil {
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "VMTemplateNotFound"), err
	}
	err = m.cliClient.PullImage(vmId)
	if err != nil {
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "VMTemplatePullFailed"), err
	}
	m.UnitsVMIDs.Store(unit.Name, vmId)
	return api.MakeStillCreatingStatus(unit.Name, unit.Image, "VMTemplatePulled"), nil
}

func (m *MacRuntime) StartContainer(unit api.Unit, spec *api.PodSpec, podName string) (*api.UnitStatus, error) {
	vmIDInterface, ok := m.UnitsVMIDs.Load(unit.Name)
	if !ok {
		return nil, fmt.Errorf("cannot find vm id for unit %s", unit.Name)
	}
	vmID := vmIDInterface.(string)
	// workaround,
	startFailure := true
	for retryCount:=0;retryCount < 5; retryCount++ {
		err := m.cliClient.Exec(vmID, []string{"echo running"}, []string{"--wait-network"})
		if err == nil {
			startFailure = false
			break
		}
	}
	if startFailure {
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "VMStartFailed"), fmt.Errorf("cannot start vm: %s", unit.Name)
	}
	//
	//unitStatus, err := m.cliClient.Start(vmID)
	//if err != nil || unitStatus.Status != AnkaStatusOK {
	//	return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "VMStartFailed"), fmt.Errorf("cannot start vm: %s", unitStatus.Message)
	//}
	started := true
	// run unit.command ?
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
	case "failed":
		return api.MakeFailedUpdateStatus(unitName, unitImage, "VMFailed"), nil
	case "running":
		startedAt, err := time.Parse(datetimeAnkaLayout, showOutput.Body.CreationDate)
		var unitState api.UnitState
		started := true
		if err != nil {
			glog.Warningf("cannot parse anka vm creation date: %v", err)
			unitState.Running = &api.UnitStateRunning{}
		} else {
			unitState.Running = &api.UnitStateRunning{StartedAt: api.Time{Time: startedAt}}
		}
		return &api.UnitStatus{
			Name:                 unitName,
			State:                unitState,
			Image:                unitImage,
			Ready:                true,
			Started:              &started,
		}, nil
	default:
		return api.MakeStillCreatingStatus(unitName, unitImage, showOutput.Message), nil
	}

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
