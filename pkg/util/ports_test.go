package util

import (
	"github.com/elotl/itzo/pkg/api"
	"testing"
)

func TestFindPortByName(t *testing.T) {
	unit := &api.Unit{
		Ports: []api.ContainerPort{
			{
				Name:          "foo",
				ContainerPort: 8080,
			},
			{
				Name:          "bar",
				ContainerPort: 9000,
			},
		},
	}
	want := 8080
	got, err := FindPortByName(unit, "foo")
	if got != want || err != nil {
		t.Errorf("Expected %v, got %v, err: %v", want, got, err)
	}
}

