package server

import (
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

// 1 full test of SyncPodUnits with mocks
