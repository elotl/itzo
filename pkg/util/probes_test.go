package util

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
)

func TestTranslateProbePorts(t *testing.T) {
	probe := &api.Probe{
		Handler: api.Handler{
			HTTPGet: &api.HTTPGetAction{
				Port: intstr.FromString("foo"),
			},
		},
	}
	unit := &api.Unit{
		Ports: []api.ContainerPort{
			{
				Name:          "foo",
				ContainerPort: 8080,
			},
		},
	}
	newProbe := TranslateProbePorts(unit, probe)
	assert.NotNil(t, newProbe)
	assert.Equal(t, intstr.Int, newProbe.HTTPGet.Port.Type)
	assert.Equal(t, int32(8080), newProbe.HTTPGet.Port.IntVal)
	assert.Equal(t, intstr.String, probe.HTTPGet.Port.Type)
	assert.Equal(t, "foo", probe.HTTPGet.Port.StrVal)

}

