package mac

import (
	"bytes"
	"fmt"
	"github.com/elotl/itzo/pkg/api"
	"github.com/stretchr/testify/assert"
	"os/exec"
	"sync"
	"testing"
)

type MockExecWrapper struct {
	RunFunc func(cmd *exec.Cmd, output *bytes.Buffer) error
}

func (m *MockExecWrapper) Run(cmd *exec.Cmd, output *bytes.Buffer) error {
	return m.RunFunc(cmd, output)
}

type RegistryMock struct {
	GetVmTemplateFunc func(vmTemplateID string) (string, error)
}

func (rm *RegistryMock) GetVMTemplate(vmTemplateID string) (string, error) {
	return rm.GetVmTemplateFunc(vmTemplateID)
}

func TestMacRuntime_ContainerStatus(t *testing.T) {
	testCases := []struct {
		name                  string
		cmdOutput             string
		unitName              string
		unitImage             string
		initialUnitVMIDsStore map[interface{}]interface{}
		expectedUnitStatus    *api.UnitStatus
	}{
		//{
		//	name:                  "vm failed",
		//	cmdOutput:             "{\"status\": \"OK\", \"body\": {\"uuid\": \"617c1ddf-5645-4947-90f0-e8f82dd1c9fb\", \"name\": \"test-vm\", \"creation_date\": \"2021-01-07T12:05:58Z\", \"cpu_cores\": 1, \"cpu_frequency\": 0, \"cpu_htt\": false, \"ram\": \"1G\", \"ram_size\": 1073741824, \"frame_buffers\": 1, \"hard_drive\": 137438953472, \"image_size\": 11014144, \"encrypted\": false, \"status\": \"failed\", \"stop_date\": \"2021-01-07T17:57:24.552773Z\"}, \"message\": \"\"}",
		//	unitName:              "dummy",
		//	unitImage:             "dummy-img",
		//	initialUnitVMIDsStore: map[interface{}]interface{}{"dummy": "617c1ddf-5645-4947-90f0-e8f82dd1c9fb"},
		//	expectedUnitStatus: &api.UnitStatus{
		//		Name: "dummy",
		//		State: api.UnitState{
		//			Waiting: &api.UnitStateWaiting{
		//				Reason:       "VMFailed",
		//				StartFailure: true,
		//			},
		//		},
		//		Image: "dummy-img",
		//	},
		//},
		//{
		//	name:                  "vm stopped",
		//	cmdOutput:             "{\"status\": \"OK\", \"body\": {\"uuid\": \"617c1ddf-5645-4947-90f0-e8f82dd1c9fb\", \"name\": \"test-vm\", \"creation_date\": \"2021-01-07T12:05:58Z\", \"cpu_cores\": 1, \"cpu_frequency\": 0, \"cpu_htt\": false, \"ram\": \"1G\", \"ram_size\": 1073741824, \"frame_buffers\": 1, \"hard_drive\": 137438953472, \"image_size\": 11014144, \"encrypted\": false, \"status\": \"stopped\", \"stop_date\": \"2021-01-07T12:05:58.040770Z\"}, \"message\": \"\"}",
		//	unitName:              "dummy",
		//	unitImage:             "dummy-img",
		//	initialUnitVMIDsStore: map[interface{}]interface{}{"dummy": "617c1ddf-5645-4947-90f0-e8f82dd1c9fb"},
		//	expectedUnitStatus: &api.UnitStatus{
		//		Name: "dummy",
		//		State: api.UnitState{
		//			Waiting: &api.UnitStateWaiting{
		//				Reason:       "VMStopped",
		//				StartFailure: true,
		//			},
		//		},
		//		Image: "dummy-img",
		//	},
		//},
		//{
		//	name:                  "vm suspended",
		//	cmdOutput:             "{\"status\": \"OK\", \"body\": {\"uuid\": \"c0847bc9-5d2d-4dbc-ba6a-240f7ff08032\", \"name\": \"10.15.7\", \"version\": \"base:port-forward-22:brew-git\", \"creation_date\": \"2020-12-23T03:35:08.270776Z\", \"cpu_cores\": 3, \"cpu_frequency\": 0, \"cpu_htt\": false, \"ram\": \"8G\", \"ram_size\": 8589934592, \"frame_buffers\": 1, \"hard_drive\": 107374182400, \"image_size\": 18381783040, \"encrypted\": false, \"addons_version\": \"2.3.1.124\", \"status\": \"suspended\", \"stop_date\": \"2020-12-23T03:40:07.109526Z\"}, \"message\": \"\"}",
		//	unitName:              "dummy",
		//	unitImage:             "dummy-img",
		//	initialUnitVMIDsStore: map[interface{}]interface{}{"dummy": "c0847bc9-5d2d-4dbc-ba6a-240f7ff08032"},
		//	expectedUnitStatus: &api.UnitStatus{
		//		Name: "dummy",
		//		State: api.UnitState{
		//			Waiting: &api.UnitStateWaiting{
		//				Reason:       "VMSuspended",
		//				StartFailure: true,
		//			},
		//		},
		//		Image: "dummy-img",
		//	},
		//},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			macRuntime := &MacRuntime{
				registryClient: nil,
				cliClient: NewAnkaCLI(func(cmd *exec.Cmd, output *bytes.Buffer) error {
					output.Write([]byte(testCase.cmdOutput))
					return nil
				}),
				UnitsVMIDs: new(sync.Map),
			}
			for k, v := range testCase.initialUnitVMIDsStore {
				macRuntime.UnitsVMIDs.Store(k, v)
			}
			unitStatus, err := macRuntime.ContainerStatus(testCase.unitName, testCase.unitImage)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedUnitStatus, unitStatus)
		})
	}
}

func TestMacRuntime_StartContainer(t *testing.T) {
	started := true
	testCases := []struct {
		name                  string
		cmdOutput             string
		unit                  api.Unit
		spec                  *api.PodSpec
		podName               string
		initialUnitVMIDsStore map[interface{}]interface{}
		expectedUnitStatus    *api.UnitStatus
		shouldErr             bool
	}{
		{
			name:      "happy path",
			cmdOutput: "{\"status\": \"OK\", \"body\": {\"uuid\": \"617c1ddf-5645-4947-90f0-e8f82dd1c9fb\", \"name\": \"test-vm\", \"creation_date\": \"2021-01-07T12:05:58Z\", \"cpu_cores\": 1, \"cpu_frequency\": 0, \"cpu_htt\": false, \"ram\": \"1G\", \"ram_size\": 1073741824, \"frame_buffers\": 1, \"hard_drive\": 137438953472, \"image_size\": 11014144, \"encrypted\": false, \"status\": \"running\", \"mac\": \"2e:9e:d9:b7:4a:c5\", \"vnc_port\": 5900, \"vnc_password\": \"admin\", \"vnc_connection_string\": \"vnc://172.31.30.149:5900\", \"pid\": 20765, \"start_date\": \"2021-01-07T17:57:23.330650Z\"}, \"message\": \"\"}",
			unit: api.Unit{
				Name:  "dummy",
				Image: "dummy-img",
			},
			spec:                  &api.PodSpec{},
			podName:               "",
			initialUnitVMIDsStore: map[interface{}]interface{}{"dummy": "617c1ddf-5645-4947-90f0-e8f82dd1c9fb"},
			expectedUnitStatus: &api.UnitStatus{
				Name: "dummy",
				State: api.UnitState{
					Running: &api.UnitStateRunning{StartedAt: api.Now()},
				},
				Image:   "dummy-img",
				Started: &started,
				Ready:   true,
			},
			shouldErr: false,
		},
		{
			name:      "error",
			cmdOutput: "{\"status\": \"ERROR\", \"body\": {}, \"message\": \"Prohibited by license or license invalid\", \"code\": 35, \"exception_type\": \"AnkaLicenseException\"}",
			unit: api.Unit{
				Name:  "dummy",
				Image: "dummy-img",
			},
			spec:                  &api.PodSpec{},
			podName:               "",
			initialUnitVMIDsStore: map[interface{}]interface{}{"dummy": "617c1ddf-5645-4947-90f0-e8f82dd1c9fb"},
			expectedUnitStatus: &api.UnitStatus{
				Name: "dummy",
				State: api.UnitState{
					Waiting: &api.UnitStateWaiting{
						Reason:       "VMStartFailed",
						StartFailure: true,
					},
				},
				Image: "dummy-img",
				Ready: false,
			},
			shouldErr: true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			macRuntime := &MacRuntime{
				registryClient: nil,
				cliClient: NewAnkaCLI(func(cmd *exec.Cmd, output *bytes.Buffer) error {
					output.Write([]byte(testCase.cmdOutput))
					return nil
				}),
				UnitsVMIDs: new(sync.Map),
			}
			for k, v := range testCase.initialUnitVMIDsStore {
				macRuntime.UnitsVMIDs.Store(k, v)
			}
			unitStatus, err := macRuntime.StartContainer(testCase.unit, testCase.spec, testCase.podName)
			if testCase.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if testCase.expectedUnitStatus.State.Running != nil {
				now := api.Now()
				testCase.expectedUnitStatus.State.Running.StartedAt = now
				unitStatus.State.Running.StartedAt = now
			}
			assert.Equal(t, testCase.expectedUnitStatus, unitStatus)
		})
	}
}

func TestMacRuntime_CreateContainerVMTemplateNotFound(t *testing.T) {
	registryClient := &RegistryMock{
		GetVmTemplateFunc: func(vmTemplateID string) (string, error) {
			return "", fmt.Errorf("err")
		},
	}
	macRuntime := &MacRuntime{
		registryClient: registryClient,
		cliClient: NewAnkaCLI(func(cmd *exec.Cmd, output *bytes.Buffer) error {
			output.Write([]byte(""))
			return nil
		}),
		UnitsVMIDs: new(sync.Map),
	}
	unitStatus, err := macRuntime.CreateContainer(api.Unit{
		Name:  "dummy",
		Image: "registry:8089/not-existing-vm-template-id",
	}, &api.PodSpec{}, "", map[string]api.RegistryCredentials{}, false)
	assert.Error(t, err)
	assert.NotNil(t, unitStatus.State.Waiting)
	assert.Equal(t, "VMTemplateNotFound", unitStatus.State.Waiting.Reason)
}

func TestMacRuntime_CreateContainerPullingFailed(t *testing.T) {
	registryClient := &RegistryMock{
		GetVmTemplateFunc: func(vmTemplateID string) (string, error) {
			return "", nil
		},
	}
	macRuntime := &MacRuntime{
		registryClient: registryClient,
		cliClient: NewAnkaCLI(func(cmd *exec.Cmd, output *bytes.Buffer) error {
			output.Write([]byte("{\"status\": \"ERROR\", \"body\": {}, \"message\": \"Pulling failed\"}"))
			return nil
		}),
		UnitsVMIDs: new(sync.Map),
	}
	unitStatus, err := macRuntime.CreateContainer(api.Unit{
		Name:  "dummy",
		Image: "registry:8089/vm-template-id",
	}, &api.PodSpec{}, "", map[string]api.RegistryCredentials{}, false)
	assert.Error(t, err)
	assert.NotNil(t, unitStatus.State.Waiting)
	assert.Equal(t, "VMTemplatePullFailed", unitStatus.State.Waiting.Reason)
}

func TestMacRuntime_CreateContainerHappyPath(t *testing.T) {
	registryClient := &RegistryMock{
		GetVmTemplateFunc: func(vmTemplateID string) (string, error) {
			return "vm-template-id", nil
		},
	}
	macRuntime := &MacRuntime{
		registryClient: registryClient,
		cliClient: NewAnkaCLI(func(cmd *exec.Cmd, output *bytes.Buffer) error {
			output.Write([]byte("{\"status\": \"OK\", \"body\": {}, \"message\": \"\"}"))
			return nil
		}),
		UnitsVMIDs: new(sync.Map),
	}
	unitStatus, err := macRuntime.CreateContainer(api.Unit{
		Name:  "dummy",
		Image: "registry:8089/vm-template-id",
	}, &api.PodSpec{}, "", map[string]api.RegistryCredentials{}, false)
	assert.NoError(t, err)
	assert.NotNil(t, unitStatus.State.Waiting)
	assert.Equal(t, "VMCreated", unitStatus.State.Waiting.Reason)
	savedVmIdInterface, ok := macRuntime.UnitsVMIDs.Load("dummy")
	assert.True(t, ok)
	savedVmId := savedVmIdInterface.(string)
	assert.Equal(t, "vm-template-id", savedVmId)
}

func TestMacRuntime_RemoveContainerHappyPath(t *testing.T) {
	registryClient := &RegistryMock{
		GetVmTemplateFunc: func(vmTemplateID string) (string, error) {
			return "", nil
		},
	}
	macRuntime := &MacRuntime{
		registryClient: registryClient,
		cliClient: NewAnkaCLI(func(cmd *exec.Cmd, output *bytes.Buffer) error {
			output.Write([]byte("{\"status\": \"OK\", \"body\": {}, \"message\": \"\"}"))
			return nil
		}),
		UnitsVMIDs: new(sync.Map),
	}
	macRuntime.UnitsVMIDs.Store("unit-name", "vm-id")
	err := macRuntime.RemoveContainer(&api.Unit{Name: "unit-name"})
	assert.NoError(t, err)
	_, ok := macRuntime.UnitsVMIDs.Load("unit-name")
	assert.False(t, ok)

}
