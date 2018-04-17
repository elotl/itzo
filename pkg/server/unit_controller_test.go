package server

import (
	"fmt"
	"testing"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/util/sets"
	"github.com/stretchr/testify/assert"
)

func TestMergeSecretsIntoSpec(t *testing.T) {
	// create secrets
	// create spec
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
	MergeSecretsIntoSpec(secrets, spec)
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
			Command: "nginx",
		},
		{
			Name:    "u2",
			Image:   "elotl/haproxy:1.4",
			Command: "haproxy",
		},
		{
			Name:    "u3",
			Image:   "elotl/useless",
			Command: "deleteme",
		},
	}
	spec := []api.Unit{
		{
			Name:    "u1",
			Image:   "elotl/nginx",
			Command: "nginx",
		},
		{
			Name:    "u2",
			Image:   "elotl/haproxy:1.5",
			Command: "haproxy",
		},
	}
	a, d := DiffUnits(spec, status, sets.NewString())
	expectedAdd := map[string]api.Unit{
		"u2": spec[1],
	}
	assert.Equal(t, expectedAdd, a)
	expectedDelete := map[string]api.Unit{
		"u2": status[1],
		"u3": status[2],
	}
	assert.Equal(t, expectedDelete, d)
}

func TestDiffUnitsWithVolumeChange(t *testing.T) {
	status := []api.Unit{
		{
			Name:    "u1",
			Image:   "elotl/nginx",
			Command: "nginx",
		},
		{
			Name:    "u2",
			Image:   "elotl/haproxy:1.4",
			Command: "haproxy",
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
			Command: "nginx",
		},
		{
			Name:    "u2",
			Image:   "elotl/haproxy:1.4",
			Command: "haproxy",
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
	expectedAdd := map[string]api.Unit{
		"u2": spec[1],
	}
	assert.Equal(t, expectedAdd, a)
	expectedDelete := map[string]api.Unit{
		"u2": status[1],
	}
	assert.Equal(t, expectedDelete, d)
}

type MountMock struct {
	Create func(*api.Volume) error
	Delete func(*api.Volume) error
	Attach func(unitname, src, dst string) error
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

type ImagePullMock struct {
	Pull func(name, image, server, username, password string) error
}

func (p *ImagePullMock) PullImage(name, image, server, username, password string) error {
	return p.Pull(name, image, server, username, password)
}

func NewImagePullMock() *ImagePullMock {
	return &ImagePullMock{
		Pull: func(name, image, server, username, password string) error {
			return nil
		},
	}
}

type UnitMock struct {
	Add    func(string, *api.Unit, []string, api.RestartPolicy) error
	Delete func(string) error
}

func (u *UnitMock) AddUnit(name string, unit *api.Unit, env []string, rp api.RestartPolicy) error {
	return u.Add(name, unit, env, rp)
}

func (u *UnitMock) DeleteUnit(name string) error {
	return u.Delete(name)
}

func NewUnitMock() *UnitMock {
	return &UnitMock{
		Add: func(name string, unit *api.Unit, env []string, rp api.RestartPolicy) error {
			return nil
		},
		Delete: func(name string) error {
			return nil
		},
	}
}

func TestFullSyncErrors(t *testing.T) {
	// pod spec and status
	// -- make something that requires volumes to change
	// and it requires units to change

	// Only the volume size has chagned
	spec := api.PodSpec{
		Units: []api.Unit{{
			Name:    "u",
			Image:   "elotl/hello",
			Command: "hello",
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
			Command: "hello",
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
		mod func(uc *UnitController)
		// This isn't the most interesting assertion but we can't
		// easily do a deep equal without recreating the exact errors
		numFailures int
	}{
		{
			mod:         func(uc *UnitController) {},
			numFailures: 0,
		},
		{
			mod: func(uc *UnitController) {
				m := uc.mountCtl.(*MountMock)
				m.Delete = func(vol *api.Volume) error {
					return fmt.Errorf("mounter failed")
				}
			},
			numFailures: 0,
		},
		{
			mod: func(uc *UnitController) {
				m := uc.mountCtl.(*MountMock)
				m.Create = func(vol *api.Volume) error {
					return fmt.Errorf("mounter failed")
				}
			},
			numFailures: 0,
		},
		{
			mod: func(uc *UnitController) {
				m := uc.unitMgr.(*UnitMock)
				m.Delete = func(name string) error {
					return fmt.Errorf("unit add failed")
				}
			},
			numFailures: 0,
		},

		// Expects failure
		{
			mod: func(uc *UnitController) {
				puller := uc.imagePuller.(*ImagePullMock)
				puller.Pull = func(name, image, server, username, password string) error {
					return fmt.Errorf("Pull Failed")
				}
			},
			numFailures: 1,
		},
		{
			mod: func(uc *UnitController) {
				m := uc.mountCtl.(*MountMock)
				m.Attach = func(unitname, src, dst string) error {
					return fmt.Errorf("mounter failed")
				}
			},
			numFailures: 1,
		},
		{
			mod: func(uc *UnitController) {
				m := uc.unitMgr.(*UnitMock)
				m.Add = func(name string, unit *api.Unit, env []string, rp api.RestartPolicy) error {
					return fmt.Errorf("unit add failed")
				}
			},
			numFailures: 1,
		},
	}

	for _, testCase := range testCases {
		unitCtl := UnitController{
			baseDir:       "/tmp/milpa",
			mountCtl:      NewMountMock(),
			unitMgr:       NewUnitMock(),
			imagePuller:   NewImagePullMock(),
			restartPolicy: api.RestartPolicyAlways,
		}
		testCase.mod(&unitCtl)
		failures := unitCtl.SyncPodUnits(&spec, &status, creds)
		assert.Len(t, failures, testCase.numFailures)
	}
}
