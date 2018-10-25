package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/util/sets"
	"github.com/stretchr/testify/assert"
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

// test diff volumes
func TestDiffVolumes(t *testing.T) {
	status := []api.Volume{
		{
			Name: "v1",
			VolumeSource: api.VolumeSource{
				EmptyDir: &api.EmptyDir{
					Medium:    api.StorageMediumMemory,
					SizeLimit: 10,
				},
			},
		},
		{
			Name: "v2",
			VolumeSource: api.VolumeSource{
				EmptyDir: &api.EmptyDir{
					Medium:    api.StorageMediumDefault, // changed to Default
					SizeLimit: 20,
				},
			},
		},
		{
			Name: "v3",
			VolumeSource: api.VolumeSource{
				EmptyDir: &api.EmptyDir{
					Medium:    api.StorageMediumMemory,
					SizeLimit: 100,
				},
			},
		},
	}
	spec := []api.Volume{
		{
			Name: "v1",
			VolumeSource: api.VolumeSource{
				EmptyDir: &api.EmptyDir{
					Medium:    api.StorageMediumMemory,
					SizeLimit: 10,
				},
			},
		},
		{
			Name: "v2",
			VolumeSource: api.VolumeSource{
				EmptyDir: &api.EmptyDir{
					Medium:    api.StorageMediumMemory,
					SizeLimit: 20,
				},
			},
		},
	}
	a, d, allMod := DiffVolumes(spec, status)
	expecedMod := sets.NewString("v2", "v3")
	assert.Equal(t, expecedMod, allMod)
	expectedAdd := map[string]api.Volume{
		"v2": spec[1],
	}
	expectedDelete := map[string]api.Volume{
		"v2": status[1],
		"v3": status[2],
	}
	assert.Equal(t, expectedAdd, a)
	assert.Equal(t, expectedDelete, d)
}

func TestDiffUnits(t *testing.T) {
	status := []api.Unit{
		{
			Name:    "u1",
			Image:   "elotl/nginx",
			Command: []string{"nginx"},
		},
		{
			Name:    "u2",
			Image:   "elotl/haproxy:1.4",
			Command: []string{"haproxy"},
		},
		{
			Name:    "u3",
			Image:   "elotl/useless",
			Command: []string{"deleteme"},
		},
	}
	spec := []api.Unit{
		{
			Name:    "u1",
			Image:   "elotl/nginx",
			Command: []string{"nginx"},
		},
		{
			Name:    "u2",
			Image:   "elotl/haproxy:1.5",
			Command: []string{"haproxy"},
		},
	}
	a, d := DiffUnits(spec, status, sets.NewString())
	expectedAdd := []api.Unit{
		spec[1],
	}
	assert.Equal(t, expectedAdd, a)
	expectedDelete := []api.Unit{
		status[1],
		status[2],
	}
	assert.Equal(t, expectedDelete, d)
}

func TestDiffUnitsWithVolumeChange(t *testing.T) {
	status := []api.Unit{
		{
			Name:    "u1",
			Image:   "elotl/nginx",
			Command: []string{"nginx"},
		},
		{
			Name:    "u2",
			Image:   "elotl/haproxy:1.4",
			Command: []string{"haproxy"},
			VolumeMounts: []api.VolumeMount{
				{
					Name: "v1",
				},
				{
					Name: "v2",
				},
			},
		},
	}
	spec := []api.Unit{
		{
			Name:    "u1",
			Image:   "elotl/nginx",
			Command: []string{"nginx"},
		},
		{
			Name:    "u2",
			Image:   "elotl/haproxy:1.4",
			Command: []string{"haproxy"},
			VolumeMounts: []api.VolumeMount{
				{
					Name: "v1",
				},
				{
					Name: "v2",
				},
			},
		},
	}
	a, d := DiffUnits(spec, status, sets.NewString("v2"))
	expectedAdd := []api.Unit{
		spec[1],
	}
	assert.Equal(t, expectedAdd, a)
	expectedDelete := []api.Unit{
		status[1],
	}
	assert.Equal(t, expectedDelete, d)
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
	Pull func(rootdir, name, image, server, username, password string) error
}

func (p *ImagePullMock) PullImage(rootdir, name, image, server, username, password string) error {
	return p.Pull(rootdir, name, image, server, username, password)
}

func NewImagePullMock() *ImagePullMock {
	return &ImagePullMock{
		Pull: func(rootdir, name, image, server, username, password string) error {
			return nil
		},
	}
}

type UnitMock struct {
	Start  func(string, string, []string, []string, []string, api.RestartPolicy) error
	Stop   func(string) error
	Remove func(string) error
}

func (u *UnitMock) StartUnit(name, workingdir string, command, args, env []string, rp api.RestartPolicy) error {
	return u.Start(name, workingdir, command, args, env, rp)
}

func (u *UnitMock) StopUnit(name string) error {
	return u.Stop(name)
}

func (u *UnitMock) RemoveUnit(name string) error {
	return u.Remove(name)
}

func NewUnitMock() *UnitMock {
	return &UnitMock{
		Start: func(name, workingdir string, command, args, env []string, rp api.RestartPolicy) error {
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
	// Only the volume size has chagned
	spec := api.PodSpec{
		Units: []api.Unit{{
			Name:    "u",
			Image:   "elotl/hello",
			Command: []string{"hello"},
			VolumeMounts: []api.VolumeMount{
				{
					Name: "v1",
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
			Image:   "elotl/hello",
			Command: []string{"hello"},
			VolumeMounts: []api.VolumeMount{
				{
					Name: "v1",
				},
			},
		}},
		Volumes: []api.Volume{{
			Name: "v1",
			VolumeSource: api.VolumeSource{
				EmptyDir: &api.EmptyDir{
					Medium:    api.StorageMediumMemory,
					SizeLimit: 10, // HERE'S OUR CHANGE
				},
			},
		}},
	}
	creds := make(map[string]api.RegistryCredentials)

	testCases := []struct {
		mod func(pc *PodController)
		// This isn't the most interesting assertion but we can't
		// easily do a deep equal without recreating the exact errors
		numFailures int
	}{
		{
			mod:         func(pc *PodController) {},
			numFailures: 0,
		},
		{
			mod: func(pc *PodController) {
				m := pc.mountCtl.(*MountMock)
				m.Delete = func(vol *api.Volume) error {
					return fmt.Errorf("mounter failed")
				}
			},
			numFailures: 0,
		},
		{
			mod: func(pc *PodController) {
				m := pc.mountCtl.(*MountMock)
				m.Create = func(vol *api.Volume) error {
					return fmt.Errorf("mounter failed")
				}
			},
			numFailures: 0,
		},
		{
			mod: func(pc *PodController) {
				m := pc.unitMgr.(*UnitMock)
				m.Stop = func(name string) error {
					return fmt.Errorf("unit stop failed")
				}
			},
			numFailures: 0,
		},
		{
			mod: func(pc *PodController) {
				m := pc.mountCtl.(*MountMock)
				m.Detach = func(name, dst string) error {
					return fmt.Errorf("mounter detach failed")
				}
			},
			numFailures: 0,
		},
		{
			mod: func(pc *PodController) {
				m := pc.unitMgr.(*UnitMock)
				m.Remove = func(name string) error {
					return fmt.Errorf("unit removal failed")
				}
			},
			numFailures: 0,
		},

		// Expects failure
		{
			mod: func(pc *PodController) {
				puller := pc.imagePuller.(*ImagePullMock)
				puller.Pull = func(rootdir, name, image, server, username, password string) error {
					return fmt.Errorf("Pull Failed")
				}
			},
			numFailures: 1,
		},
		{
			mod: func(pc *PodController) {
				m := pc.mountCtl.(*MountMock)
				m.Attach = func(unitname, src, dst string) error {
					return fmt.Errorf("mounter failed")
				}
			},
			numFailures: 1,
		},
		{
			mod: func(pc *PodController) {
				m := pc.unitMgr.(*UnitMock)
				m.Start = func(name, workingdir string, command, args, env []string, rp api.RestartPolicy) error {
					return fmt.Errorf("unit add failed")
				}
			},
			numFailures: 1,
		},
	}

	for _, testCase := range testCases {
		podCtl := PodController{
			rootdir:     "/tmp/milpa/units",
			mountCtl:    NewMountMock(),
			unitMgr:     NewUnitMock(),
			imagePuller: NewImagePullMock(),
			syncErrors:  make(map[string]api.UnitStatus),
		}
		testCase.mod(&podCtl)
		podCtl.SyncPodUnits(&spec, &status, creds)
		podCtl.waitGroup.Wait()
		assert.Len(t, podCtl.syncErrors, testCase.numFailures)
	}
}

func createTestUnits(names []string) (string, []*Unit, func()) {
	tmpdir, err := ioutil.TempDir("", "itzo-test")
	if err != nil {
		panic(err)
	}
	units := make([]*Unit, len(names))
	for i, name := range names {
		u, err := OpenUnit(tmpdir, name)
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
	rootdir, units, closer := createTestUnits([]string{myUnit.Name, initUnit.Name})
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

	podCtl := NewPodController(rootdir, nil, nil, nil)
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
