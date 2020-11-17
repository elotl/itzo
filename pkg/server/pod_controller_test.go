/*
Copyright 2020 Elotl Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"fmt"
	"github.com/elotl/itzo/pkg/logbuf"
	runtime2 "github.com/elotl/itzo/pkg/runtime"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/unit"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestMergeSecretsIntoSpec(t *testing.T) {
	spec := &api.PodSpec{
		Units: []api.Unit{
			{
				Env: []api.EnvVar{
					{
						Name:  "foo",
						Value: "fooval",
					},
					{
						Name: "bar",
						ValueFrom: &api.EnvVarSource{
							SecretKeyRef: &api.SecretKeySelector{
								Name: "name1",
								Key:  "value1",
							},
						},
					},
					{
						Name: "baz",
						ValueFrom: &api.EnvVarSource{
							SecretKeyRef: &api.SecretKeySelector{
								Name: "name2",
								Key:  "value2",
							},
						},
					},
				},
			},
		},
	}
	secrets := map[string]map[string][]byte{
		"name1": map[string][]byte{
			"value1": []byte("secret1"),
		},
	}
	MergeSecretsIntoSpec(secrets, spec.Units)
	assert.Len(t, spec.Units[0].Env, 2)
	assert.Equal(t, api.EnvVar{"foo", "fooval", nil}, spec.Units[0].Env[0])
	assert.Equal(t, api.EnvVar{"bar", "secret1", nil}, spec.Units[0].Env[1])
}

func TestUnitsSlicesEqual(t *testing.T) {
	testCases := []struct {
		name           string
		specUnits      []api.Unit
		statusUnits    []api.Unit
		expectedResult bool
	}{
		{
			"image version changed",
			[]api.Unit{
				api.Unit{
					Image: "elotl-img:v2",
				},
			},
			[]api.Unit{
				api.Unit{
					Image: "elotl-img:v1",
				},
			},
			false,
		},
		{
			name: "unit removed",
			specUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
			},
			statusUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
				api.Unit{
					Image: "elotl-img-2",
				},
			},
			expectedResult: false,
		},
		{
			name: "unit added",
			specUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
				api.Unit{
					Image: "elotl-img-2",
				},
			},
			statusUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
			},
			expectedResult: false,
		},
		{
			name: "no changes",
			specUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
			},
			statusUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
			},
			expectedResult: true,
		},
		{
			name: "different order",
			specUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
				api.Unit{
					Image: "elotl-img2",
				},
			},
			statusUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img2",
				},
				api.Unit{
					Image: "elotl-img1",
				},
			},
			expectedResult: false,
		},
		{
			name: "different order, same images",
			specUnits: []api.Unit{
				api.Unit{
					Name:  "unit1",
					Image: "elotl-img1",
				},
				api.Unit{
					Name:  "unit2",
					Image: "elotl-img1",
				},
			},
			statusUnits: []api.Unit{
				api.Unit{
					Name:  "unit2",
					Image: "elotl-img1",
				},
				api.Unit{
					Name:  "unit1",
					Image: "elotl-img1",
				},
			},
			expectedResult: false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := unitsSlicesEqual(testCase.specUnits, testCase.statusUnits)
			assert.Equal(t, testCase.expectedResult, result)
		})
	}

}

func TestDiffUnits(t *testing.T) {
	testCases := []struct {
		name              string
		specUnits         []api.Unit
		statusUnits       []api.Unit
		expectedDiffCount int
		expectedToAdd     []api.Unit
		expectedToDelete  []api.Unit
	}{
		{
			"image version changed",
			[]api.Unit{
				api.Unit{
					Image: "elotl-img:v2",
				},
			},
			[]api.Unit{
				api.Unit{
					Image: "elotl-img:v1",
				},
			},
			2,
			[]api.Unit{
				api.Unit{
					Image: "elotl-img:v2",
				},
			},
			[]api.Unit{
				api.Unit{
					Image: "elotl-img:v1",
				},
			},
		},
		{
			name: "unit removed",
			specUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
			},
			statusUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
				api.Unit{
					Image: "elotl-img-2",
				},
			},
			expectedDiffCount: 1,
			expectedToAdd:     []api.Unit{},
			expectedToDelete: []api.Unit{
				api.Unit{
					Image: "elotl-img-2",
				},
			},
		},
		{
			name: "unit added",
			specUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
				api.Unit{
					Image: "elotl-img-2",
				},
			},
			statusUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
			},
			expectedDiffCount: 1,
			expectedToAdd: []api.Unit{
				api.Unit{
					Image: "elotl-img-2",
				},
			},
			expectedToDelete: []api.Unit{},
		},
		{
			name: "no changes",
			specUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
			},
			statusUnits: []api.Unit{
				api.Unit{
					Image: "elotl-img1",
				},
			},
			expectedDiffCount: 0,
			expectedToAdd:     []api.Unit{},
			expectedToDelete:  []api.Unit{},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			toAdd, toDelete := diffUnits(testCase.specUnits, testCase.statusUnits)
			diffCount := len(toAdd) + len(toDelete)
			assert.Equal(t, testCase.expectedDiffCount, diffCount)
			assert.Equal(t, testCase.expectedToAdd, toAdd)
			assert.Equal(t, testCase.expectedToDelete, toDelete)

		})
	}
}

type MountMock struct {
	Create func(*api.Volume) error
	Delete func(*api.Volume) error
	Attach func(unitname, src, dst string) error
	Detach func(unitname, dst string) error
}

func NewMountMock() *MountMock {
	return &MountMock{
		Create: func(vol *api.Volume) error {
			return nil
		},
		Delete: func(vol *api.Volume) error {
			return nil
		},
		Attach: func(unitname, src, dst string) error {
			return nil
		},
		Detach: func(unitname, dst string) error {
			return nil
		},
	}
}

func (m *MountMock) CreateMount(vol *api.Volume) error {
	return m.Create(vol)
}

func (m *MountMock) DeleteMount(vol *api.Volume) error {
	return m.Delete(vol)
}

func (m *MountMock) AttachMount(unitname, src, dst string) error {
	return m.Attach(unitname, src, dst)
}

func (m *MountMock) DetachMount(unitname, dst string) error {
	return m.Detach(unitname, dst)
}

type ImagePullMock struct {
	Pull func(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials) error
}

func (p *ImagePullMock) ListImages() {
	panic("implement me")
}

func (p *ImagePullMock) ImageStatus(rootdir, image string) error {
	panic("implement me")
}

func (p *ImagePullMock) RemoveImage(rootdir, image string) error {
	panic("implement me")
}

func (p *ImagePullMock) PullImage(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials) error {
	return p.Pull(rootdir, name, image, registryCredentials)
}

func NewImagePullMock() *ImagePullMock {
	return &ImagePullMock{
		Pull: func(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials) error {
			return nil
		},
	}
}

type UnitMock struct {
	Start  func(string, string, string, string, string, []string, []string, []string, api.RestartPolicy) error
	Stop   func(string) error
	Remove func(string) error
}

func (u *UnitMock) UnitRunning(s string) bool {
	return false
}

func (u *UnitMock) GetLogBuffer(unitName string) (*logbuf.LogBuffer, error) {
	panic("implement me")
}

func (u *UnitMock) ReadLogBuffer(unitName string, n int) ([]logbuf.LogEntry, error) {
	panic("implement me")
}

func (u *UnitMock) GetPid(s string) (int, bool) {
	panic("implement me")
}

func (u *UnitMock) StartUnit(podname, hostname, unitname, workingdir, netns string, command, args, env []string, rp api.RestartPolicy) error {
	return u.Start(podname, hostname, unitname, workingdir, netns, command, args, env, rp)
}

func (u *UnitMock) StopUnit(name string) error {
	return u.Stop(name)
}

func (u *UnitMock) RemoveUnit(name string) error {
	return u.Remove(name)
}

func NewUnitMock() *UnitMock {
	return &UnitMock{
		Start: func(pod, hostname, name, workingdir, netns string, command, args, env []string, rp api.RestartPolicy) error {
			return nil
		},
		Stop: func(name string) error {
			return nil
		},
		Remove: func(name string) error {
			return nil
		},
	}
}

// Here we're testing 1. that we do the diffs somewhat correctly
// and that we generate the correct number of errors when things
// fail.
func TestFullSyncErrors(t *testing.T) {
	// Only the unit image has chagned
	spec := api.PodSpec{
		Units: []api.Unit{{
			Name:    "u",
			Image:   "elotl/hello",
			Command: []string{"hello"},
			VolumeMounts: []api.VolumeMount{
				{
					Name:    "v1",
					SubPath: "",
				},
			},
		}},
		Volumes: []api.Volume{{
			Name: "v1",
			VolumeSource: api.VolumeSource{
				EmptyDir: &api.EmptyDir{
					Medium:    api.StorageMediumMemory,
					SizeLimit: 20,
				},
			},
		}},
	}

	status := api.PodSpec{
		Units: []api.Unit{{
			Name:    "u",
			Image:   "elotl/goodbye", // HERE'S OUR CHANGE
			Command: []string{"hello"},
			VolumeMounts: []api.VolumeMount{
				{
					Name:    "v1",
					SubPath: "",
				},
			},
		}},
		Volumes: []api.Volume{{
			Name: "v1",
			VolumeSource: api.VolumeSource{
				EmptyDir: &api.EmptyDir{
					Medium:    api.StorageMediumMemory,
					SizeLimit: 20,
				},
			},
		}},
	}
	creds := make(map[string]api.RegistryCredentials)

	testCases := []struct {
		name string
		mod  func(pc *PodController)
		// This isn't the most interesting assertion but we can't
		// easily do a deep equal without recreating the exact errors
		numFailures int
	}{
		{
			name:        "happy_path",
			mod:         func(pc *PodController) {},
			numFailures: 0,
		},
		{
			name: "mount_delete_failed",
			mod: func(pc *PodController) {
				r := pc.runtime.(*runtime2.ItzoRuntime)
				m := r.MountCtl.(*MountMock)
				m.Delete = func(vol *api.Volume) error {
					return fmt.Errorf("mounter failed")
				}
			},
			numFailures: 0,
		},
		{
			name: "mount_create_failed",
			mod: func(pc *PodController) {
				r := pc.runtime.(*runtime2.ItzoRuntime)
				m := r.MountCtl.(*MountMock)
				m.Create = func(vol *api.Volume) error {
					return fmt.Errorf("mounter failed")
				}
			},
			numFailures: 0,
		},
		{
			name: "unit_stop_failed",
			mod: func(pc *PodController) {
				r := pc.runtime.(*runtime2.ItzoRuntime)
				m := r.UnitMgr.(*UnitMock)
				m.Stop = func(name string) error {
					return fmt.Errorf("unit stop failed")
				}
			},
			numFailures: 0,
		},
		{
			name: "mount_detach_failed",
			mod: func(pc *PodController) {
				r := pc.runtime.(*runtime2.ItzoRuntime)
				m := r.MountCtl.(*MountMock)
				m.Detach = func(name, dst string) error {
					return fmt.Errorf("mounter detach failed")
				}
			},
			numFailures: 0,
		},
		{
			name: "unit_remove_failed",
			mod: func(pc *PodController) {
				r := pc.runtime.(*runtime2.ItzoRuntime)
				m := r.UnitMgr.(*UnitMock)
				m.Remove = func(name string) error {
					return fmt.Errorf("unit removal failed")
				}
			},
			numFailures: 0,
		},

		// Expects failure
		{
			name: "pull_failed",
			mod: func(pc *PodController) {
				r := pc.runtime.(*runtime2.ItzoRuntime)
				puller := r.ImgPuller.(*ImagePullMock)
				puller.Pull = func(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials) error {
					return fmt.Errorf("Pull Failed")
				}
			},
			numFailures: 1,
		},
		{
			name: "attach_failed",
			mod: func(pc *PodController) {
				r := pc.runtime.(*runtime2.ItzoRuntime)
				m := r.MountCtl.(*MountMock)
				m.Attach = func(unitname, src, dst string) error {
					return fmt.Errorf("mounter failed")
				}
			},
			numFailures: 1,
		},
		{
			name: "unit_start_failed",
			mod: func(pc *PodController) {
				r := pc.runtime.(*runtime2.ItzoRuntime)
				m := r.UnitMgr.(*UnitMock)
				m.Start = func(pod, hostname, name, workingdir, netns string, command, args, env []string, rp api.RestartPolicy) error {
					return fmt.Errorf("unit add failed")
				}
			},
			numFailures: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			runtime := runtime2.NewItzoRuntime(DEFAULT_ROOTDIR, NewUnitMock(), NewMountMock(), NewImagePullMock())

			podCtl := PodController{
				rootdir:    DEFAULT_ROOTDIR,
				runtime:    runtime,
				syncErrors: make(map[string]api.UnitStatus),
			}
			testCase.mod(&podCtl)
			podCtl.SyncPodUnits(&spec, &status, creds)
			podCtl.waitGroup.Wait()
			assert.Len(t, podCtl.syncErrors, testCase.numFailures)
		})
	}
}

func TestPodController_SyncPodUnits(t *testing.T) {
	testCases := []struct {
		name                 string
		spec                 *api.PodSpec
		status               *api.PodSpec
		expectedRestartCount int32
		expectedEvent        string
	}{
		{
			"init units changed",
			&api.PodSpec{
				InitUnits: []api.Unit{
					api.Unit{
						Name:  "unit1",
						Image: "img-1",
					},
					api.Unit{
						Name:  "unit2",
						Image: "img-2",
					},
				},
			},
			&api.PodSpec{
				InitUnits: []api.Unit{
					api.Unit{
						Name:  "unit1",
						Image: "img-1",
					},
					api.Unit{
						Name:  "unit2",
						Image: "img-4",
					},
				},
			},
			1,
			"pod_restart",
		},
		{
			"nothing changed",
			&api.PodSpec{
				Units: []api.Unit{
					api.Unit{
						Name:  "unit1",
						Image: "img",
					},
				},
			},
			&api.PodSpec{
				Units: []api.Unit{
					api.Unit{
						Name:  "unit1",
						Image: "img",
					},
				},
			},
			0,
			"no_changes",
		},
		{
			"unit image changed",
			&api.PodSpec{
				InitUnits: []api.Unit{},
				Units: []api.Unit{
					api.Unit{
						Image: "img:1",
					},
				},
			},
			&api.PodSpec{
				InitUnits: []api.Unit{},
				Units: []api.Unit{
					api.Unit{
						Image: "img:2",
					},
				},
			},
			0,
			"units_changed",
		},
		{
			"pod created",
			&api.PodSpec{
				InitUnits: []api.Unit{},
				Units: []api.Unit{
					api.Unit{
						Image: "img:1",
					},
				},
			},
			&api.PodSpec{
				Phase:         api.PodRunning,
				RestartPolicy: api.RestartPolicyAlways,
			},
			0,
			"pod_created",
		},
	}
	creds := make(map[string]api.RegistryCredentials)
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			runtime := runtime2.NewItzoRuntime(DEFAULT_ROOTDIR, NewUnitMock(), NewMountMock(), NewImagePullMock())
			pc := PodController{
				rootdir:    DEFAULT_ROOTDIR,
				runtime:    runtime,
				syncErrors: make(map[string]api.UnitStatus),
			}
			event := pc.SyncPodUnits(testCase.spec, testCase.status, creds)
			assert.Equal(t, testCase.expectedEvent, event)
			assert.Equal(t, testCase.expectedRestartCount, pc.podRestartCount)
		})
	}
}

func createTestUnits(names ...string) (string, []*unit.Unit, func()) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	if err != nil {
		panic(err)
	}
	units := make([]*unit.Unit, len(names))
	for i, name := range names {
		u, err := unit.OpenUnit(tmpdir, name)
		if err != nil {
			panic(err)
		}
		units[i] = u
	}
	closer := func() {
		for _, u := range units {
			u.Destroy()
		}
		os.RemoveAll(tmpdir)
	}
	return tmpdir, units, closer
}

func assertStatusEqual(t *testing.T, expected, actual *api.UnitStatus) {
	assert.Equal(t, expected.Name, actual.Name)
	assert.Equal(t, expected.RestartCount, actual.RestartCount)
	assert.Equal(t, expected.Image, actual.Image)
	if expected.State.Waiting != nil && actual.State.Waiting != nil {
		assert.Equal(t, expected.State, actual.State)
		return
	}
	if expected.State.Running != nil && actual.State.Running != nil {
		return
	}
	if expected.State.Terminated != nil && actual.State.Terminated != nil {

		assert.Equal(t, expected.State.Terminated.ExitCode, actual.State.Terminated.ExitCode)
		return
	}
	t.Errorf("Statuses are in different states:\nExpected: %+v\nActual: %+v", expected, actual)
}

func TestPodControllerStatus(t *testing.T) {
	myUnit := api.Unit{
		Name:    "foounit",
		Image:   "elotl/foo",
		Command: []string{"runfoo"},
	}
	initUnit := api.Unit{
		Name:    "initunit",
		Image:   "elotl/init",
		Command: []string{"runinit"},
	}
	rootdir, units, closer := createTestUnits(myUnit.Name, initUnit.Name)
	defer closer()
	status := api.PodSpec{
		Units:     []api.Unit{myUnit},
		InitUnits: []api.Unit{initUnit},
	}
	running := api.UnitState{
		Running: &api.UnitStateRunning{
			StartedAt: api.Now(),
		},
	}
	succeeded := api.UnitState{
		Running: &api.UnitStateRunning{
			StartedAt: api.Now(),
		},
	}
	err := units[0].SetImage(myUnit.Image)
	assert.NoError(t, err)
	err = units[0].SetState(running, nil)
	assert.NoError(t, err)
	err = units[1].SetImage(initUnit.Image)
	assert.NoError(t, err)
	err = units[1].SetState(succeeded, nil)
	assert.NoError(t, err)
	expected := api.UnitStatus{
		Name:  myUnit.Name,
		State: running,
		Image: myUnit.Image,
	}
	initExpected := api.UnitStatus{
		Name:  initUnit.Name,
		State: succeeded,
		Image: initUnit.Image,
	}
	s, err := units[0].GetStatus()
	assert.NoError(t, err)
	assertStatusEqual(t, &expected, s)
	s, err = units[1].GetStatus()
	assert.NoError(t, err)
	assertStatusEqual(t, &initExpected, s)
	runtime := runtime2.NewItzoRuntime(rootdir, unit.NewUnitManager(rootdir), nil, nil)
	podCtl := &PodController{
		rootdir:    rootdir,
		runtime:    runtime,
		usePodman:  false,
		syncErrors: make(map[string]api.UnitStatus),
	}
	podCtl.podStatus = &status
	statuses, initStatuses, err := podCtl.GetStatus()
	assert.NoError(t, err)
	assert.Len(t, statuses, 1)
	assertStatusEqual(t, &expected, &statuses[0])
	assert.Len(t, initStatuses, 1)
	assertStatusEqual(t, &initExpected, &initStatuses[0])

	// now overwrite the status with a failure
	// make sure it's overwritten in the status
	expected.State = api.UnitState{
		Waiting: &api.UnitStateWaiting{
			StartFailure: true,
		},
	}
	podCtl.syncErrors[myUnit.Name] = expected
	statuses, initStatuses, err = podCtl.GetStatus()
	assert.NoError(t, err)
	assert.Len(t, statuses, 1)
	assertStatusEqual(t, &expected, &statuses[0])
	assert.Len(t, initStatuses, 1)
	assertStatusEqual(t, &initExpected, &initStatuses[0])
}

func TestWaitForInitUnitReturnCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		exitCode      int32
		restartPolicy api.RestartPolicy
		success       bool
		contextDone   bool
	}{
		{
			exitCode:      int32(0),
			restartPolicy: api.RestartPolicyNever,
			success:       true,
			contextDone:   false,
		},
		{
			exitCode:      int32(0),
			restartPolicy: api.RestartPolicyOnFailure,
			success:       true,
			contextDone:   false,
		},
		{
			exitCode:      int32(1),
			restartPolicy: api.RestartPolicyNever,
			success:       false,
			contextDone:   false,
		},
		{
			exitCode:      int32(1),
			restartPolicy: api.RestartPolicyOnFailure,
			success:       false,
			contextDone:   true,
		},
	}
	waitForInitUnitPollInterval = 1 * time.Millisecond
	for i, tc := range tests {
		msg := fmt.Sprintf("Test case %d", i)
		rootDir, units, closer := createTestUnits("testunit")
		runtime := runtime2.NewItzoRuntime(rootDir, NewUnitMock(), NewMountMock(), NewImagePullMock())
		podCtl := PodController{
			rootdir:    rootDir,
			runtime:    runtime,
			syncErrors: make(map[string]api.UnitStatus),
		}

		u := units[0]
		err := u.SetState(api.UnitState{
			Terminated: &api.UnitStateTerminated{
				ExitCode: tc.exitCode,
			},
		}, nil)
		assert.NoError(t, err, msg)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		retVal := podCtl.waitForInitUnit(ctx, u.Name, u.Image, tc.restartPolicy)
		select {
		case <-ctx.Done():
			assert.True(t, tc.contextDone, msg)
		default:
			assert.False(t, tc.contextDone, msg)
		}
		assert.Equal(t, tc.success, retVal, msg)
		closer()
		cancel()
	}
}
