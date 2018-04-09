package server

import (
	"os"

	"github.com/elotl/itzo/pkg/api"
)

var specChanSize = 100

// I know how to do one thing: Make Controllers. A fuckload of controllers...
type UnitController struct {
	desired  *api.PodSpec
	current  *api.PodSpec
	procs    map[string]*os.Process
	specChan chan *api.PodSpec
}

type registryCredentials struct {
	server   string
	username string
	password string
}

type PodParameters struct {
	secrets map[string]map[string]string
	creds   map[string]registryCredentials
	spec    api.PodSpec
}

func MakeUnitController() *UnitController {
	uc := UnitController{
		procs:    make(map[string]*os.Process),
		specChan: make(chan *api.PodSpec, specChanSize),
	}
	return &uc
}

func (uc *UnitController) UpdatePod(ps *api.PodSpec) {

}

func (uc *UnitController) Start() {
	for {
		uc.desired = <-uc.specChan
		uc.syncSpec()
	}
}

func (uc *UnitController) SyncSpec() {
	// diff volumes?
	// diff units

}
