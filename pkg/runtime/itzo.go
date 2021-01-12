package runtime

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
)

// TODO: move itzo runtime to separate package

type Puller interface {
	PullImage(rootdir, name, image, server, username, password string) error
}

type Mounter interface {
	CreateMount(*api.Volume) error
	DeleteMount(*api.Volume) error
	AttachMount(unitname, src, dst string) error
	DetachMount(unitname, dst string) error
}

// Too bad there isn't a word for a creator AND destroyer
// Coulda gone with Shiva(er) but that's a bit imprecise...
type UnitRunner interface {
	StartUnit(string, string, string, string, string, []string, []string, []string, api.RestartPolicy) error
	StopUnit(string) error
	RemoveUnit(string) error
	UnitRunning(string) bool
	GetLogBuffer(unitName string) (*logbuf.LogBuffer, error)
	ReadLogBuffer(unitName string, n int) ([]logbuf.LogEntry, error)
	GetPid(string) (int, bool)
}

