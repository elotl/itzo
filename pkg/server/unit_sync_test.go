package server

import (
	"testing"

	"github.com/elotl/itzo/pkg/api"
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
