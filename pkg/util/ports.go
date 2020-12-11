package util

import (
	"fmt"
	"github.com/elotl/itzo/pkg/api"
)

// findPortByName is a helper function to look up a port in a container by name.
func FindPortByName(unit *api.Unit, portName string) (int, error) {
	for _, port := range unit.Ports {
		if port.Name == portName {
			return int(port.ContainerPort), nil
		}
	}
	return 0, fmt.Errorf("port %s not found", portName)
}

