package unit

import (
	"context"
	"fmt"
	"github.com/containers/libpod/v2/pkg/bindings"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"os"
)

type UnitRunner interface {
	StartUnit(string, string, string, string, string, []string, []string, []string, api.RestartPolicy) error
	StopUnit(string) error
	RemoveUnit(string) error
}

type PodmanManager struct {}

func NewPodmanManager() *PodmanManager {
	// Get Podman socket location
	sock_dir := os.Getenv("XDG_RUNTIME_DIR")
	socket := "unix:" + sock_dir + "/podman/podman.sock"

	// Connect to Podman socket
	connText, err := bindings.NewConnection(context.Background(), socket)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return &PodmanManager{}
}

func (pm *PodmanManager) StartUnit(string, string, string, string, string, []string, []string, []string, api.RestartPolicy) error {
	return nil
}

func (pm *PodmanManager) StopUnit(name string) error {
	return nil
}

func (pm *PodmanManager) RemoveUnit(name string) error  {
	return nil
}

func (pm *PodmanManager) GetLogBuffer(unit string) (*logbuf.LogBuffer, error) {
	return nil, nil
}

func (pm *PodmanManager) ReadLogBuffer(unit string, n int) ([]logbuf.LogEntry, error) {
	return nil, nil
}

func (pm *PodmanManager) UnitRunning(unit string) bool {
	return true
}

func (pm *PodmanManager) GetPid(unitName string) (int, bool)  {
	return 0, false
}



