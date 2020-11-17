/*
Copyright 2020 Elotl Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"fmt"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/mount"
	"github.com/elotl/itzo/pkg/runtime"
	"sync"
	"time"

	"github.com/elotl/itzo/pkg/api"
	itzounit "github.com/elotl/itzo/pkg/unit"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

const (
	UpdateTypeNoChanges   = "no_changes"
	UpdateTypePodRestart  = "pod_restart"
	UpdateTypePodCreate   = "pod_created"
	UpdateTypeUnitsChange = "units_changed"
)

var (
	specChanSize                = 100
	waitForInitUnitPollInterval = 1 * time.Second
)

// I know how to do one thing: Make Controllers. A ton of controllers...
type PodController struct {
	rootdir     string
	podName     string
	podHostname string
	runtime     runtime.RuntimeService
	podStatus   *api.PodSpec
	updateChan  chan *api.PodParameters
	allCreds   map[string]api.RegistryCredentials
	// We keep syncErrors in the map between syncs until a sync works
	// and we clear or overwrite the error
	syncErrors map[string]api.UnitStatus
	cancelFunc context.CancelFunc
	waitGroup  sync.WaitGroup
	netNS      string
	podIP      string
	podRestartCount int32
	usePodman  bool
}

func NewPodController(rootdir string, usePodman bool) (*PodController, error) {
	var podRuntime runtime.RuntimeService
	if usePodman {
		podRuntime, _ = runtime.NewPodmanRuntime()
		glog.Info("using podman image puller")
	} else {
		mounter := mount.NewOSMounter(rootdir)
		unitMgr := itzounit.NewUnitManager(rootdir)
		imgPuller := runtime.ImagePuller{}
		podRuntime = runtime.NewItzoRuntime(rootdir, unitMgr, mounter, &imgPuller)
	}
	return &PodController{
		rootdir:     rootdir,
		runtime: 	 podRuntime,
		updateChan:  make(chan *api.PodParameters, specChanSize),
		syncErrors:  make(map[string]api.UnitStatus),
		podStatus: &api.PodSpec{
			Phase:         api.PodRunning,
			RestartPolicy: api.RestartPolicyAlways,
		},
		cancelFunc: nil,
		podRestartCount: 0,
		usePodman: usePodman,
	}, nil
}

func (pc *PodController) SetPodNetwork(netNS, podIP string) {
	pc.netNS = netNS
	pc.podIP = podIP
	pc.runtime.SetPodNetwork(netNS, podIP)
}

func (pc *PodController) runUpdateLoop() {
	for {
		// pull updates off until we have no more updates since we
		// only care about the latest update
		if len(pc.updateChan) > 1 {
			<-pc.updateChan
			continue
		}
		podParams := <-pc.updateChan
		glog.Infof("New pod update")
		pc.podName = podParams.PodName
		pc.podHostname = podParams.PodHostname
		spec := &podParams.Spec
		MergeSecretsIntoSpec(podParams.Secrets, spec.Units)
		MergeSecretsIntoSpec(podParams.Secrets, spec.InitUnits)
		pc.SyncPodUnits(spec, pc.podStatus, podParams.Credentials)
		pc.podStatus = spec
	}
}

func (pc *PodController) Start() {
	go pc.runUpdateLoop()
}

func (pc *PodController) GetUnitName(unitName string) (string, error) {
	if unitName == "" {
		if len(pc.podStatus.Units) > 0 {
			return pc.podStatus.Units[0].Name, nil
		} else {
			return "", fmt.Errorf("No running units on pod")
		}
	}
	return unitName, nil
}

func (pc *PodController) UpdatePod(params *api.PodParameters) error {
	// If something goes horribly wrong, don't block the rest client,
	// just return an error for the update and kick the problem back
	// to the milpa server
	if len(pc.updateChan) == specChanSize {
		return fmt.Errorf("Error updating pod spec: too many pending updates")
	}
	pc.updateChan <- params
	return nil
}

// Modifies the PodSpec and inserts secrets into the spec
func MergeSecretsIntoSpec(secrets map[string]map[string][]byte, units []api.Unit) {
	for i := 0; i < len(units); i++ {
		newEnv := make([]api.EnvVar, 0, len(units[i].Env))
		for _, ev := range units[i].Env {
			if ev.ValueFrom == nil {
				newEnv = append(newEnv, ev)
			} else if ev.ValueFrom.SecretKeyRef != nil {
				name := ev.ValueFrom.SecretKeyRef.Name
				key := ev.ValueFrom.SecretKeyRef.Key
				m, exists := secrets[name]
				if !exists {
					glog.Errorf("Could not find secret for env var %s at %s[%s]",
						ev.Name, name, key)
					continue
				}
				val, exists := m[key]
				if !exists {
					glog.Errorf("Could not find secret for env var %s at %s[%s]",
						ev.Name, name, key)
					continue
				}
				newEnv = append(newEnv, api.EnvVar{Name: ev.Name, Value: string(val)})
			}
		}
		units[i].Env = newEnv
	}
}

func unitsEqual(specUnit, statusUnit api.Unit) bool {
	if specUnit.Image == statusUnit.Image && specUnit.Name == statusUnit.Name {
		return true
	}
	return false
}

func unitsSlicesEqual(specUnits []api.Unit, statusUnits []api.Unit) bool {
	// Changes to the init container spec are limited to the container image field.
	// Altering an init container image field is equivalent to restarting the Pod.
	// https://kubernetes.io/docs/concepts/workloads/pods/init-containers/#detailed-behavior
	if len(specUnits) != len(statusUnits) {
		return false
	}
	for i := range specUnits {
		if !unitsEqual(specUnits[i], statusUnits[i]) {
			return false
		}
	}
	return true
}


func diffUnits(spec []api.Unit, status []api.Unit) ([]api.Unit, []api.Unit) {
	toAdd := make([]api.Unit, 0)
	toDelete := make([]api.Unit, 0)
	if len(spec) >= len(status) {
		for i, unit := range spec {
			if i >= len(status) {
				toAdd = append(toAdd, unit)
				continue
			}
			if !unitsEqual(unit, status[i]) {
				toDelete = append(toDelete, status[i])
				toAdd = append(toAdd, unit)
			}
		}

	} else {
		for i, unit := range status {
			if i >= len(spec) {
				toDelete = append(toDelete, unit)
				continue
			}
			if !unitsEqual(unit, spec[i]) {
				toDelete = append(toDelete, unit)
				toAdd = append(toAdd, spec[i])
			}
		}
	}
	return toAdd, toDelete
}

func detectChangeType(spec *api.PodSpec, status *api.PodSpec) string {
	if !unitsSlicesEqual(spec.InitUnits, status.InitUnits) {
		return UpdateTypePodRestart
	}
	if len(status.Units) == 0 && len(status.InitUnits) == 0 {
		return UpdateTypePodCreate
	}
	toAdd, toDelete := diffUnits(spec.Units, status.Units)
	diffSize := len(toAdd) + len(toDelete)

	if diffSize > 0 {
		return UpdateTypeUnitsChange
	}
	return UpdateTypeNoChanges
}

func (pc *PodController) SyncPodUnits(spec *api.PodSpec, status *api.PodSpec, allCreds map[string]api.RegistryCredentials) string {
	// By this point, spec must have had the secrets merged into the env vars
	glog.Info("syncing pod units...")
	pc.allCreds = allCreds
	event := detectChangeType(spec, status)
	glog.Infof("detected change: %s", event)
	var initsToStart []api.Unit
	var unitsToStart []api.Unit
	switch event {
	case UpdateTypeNoChanges:
		// there aren't any units to restart
		return event
	case UpdateTypeUnitsChange:
		addUnits, err := pc.RestartUnits(spec, status)
		if err != nil {
			glog.Errorf("error restarting units: %v", err)
			return event
		}
		initsToStart, unitsToStart = []api.Unit{}, addUnits
	case UpdateTypePodCreate:
		// start pod
		err := pc.CreatePod(spec)
		if err != nil {
			glog.Errorf("error creating pod: %v", err)
			return event
		}
		initsToStart, unitsToStart = spec.InitUnits, spec.Units
	case UpdateTypePodRestart:
		err := pc.RestartPod(spec, status)
		if err != nil {
			glog.Errorf("error restarting pod: %v", err)
			return event
		}
		initsToStart = spec.InitUnits
		unitsToStart = spec.Units
	}
	_, cancel := context.WithCancel(context.Background())
	if pc.cancelFunc != nil {
		glog.Infof("Canceling previous pod update")
		pc.cancelFunc()
	}
	pc.cancelFunc = cancel
	pc.waitGroup.Wait() // Wait for previous update to finish.
	pc.waitGroup = sync.WaitGroup{}
	pc.waitGroup.Add(1)
	go func() {
		ipolicy := spec.RestartPolicy
		if ipolicy == api.RestartPolicyAlways {
			// Restart policy "Always" is nonsensical for init units.
			ipolicy = api.RestartPolicyOnFailure
		}
		for _, unit := range initsToStart {
			// Start init units first, one by one, and wait for each to finish.
			unitStatus, err := pc.runtime.StartContainer(unit, spec, pc.podName)
			if err != nil {
				glog.Errorf("error starting unit %s : %v", unit.Name, err)
				pc.syncErrors[unit.Name] = *unitStatus
				pc.waitGroup.Done()
				return
			}
			// TODO rethink
			//if !pc.waitForInitUnit(ctx, unit.Name, ipolicy) {
			//	return
			//}
		}
		for _, unit := range unitsToStart {
			unitStatus, err := pc.runtime.StartContainer(unit, spec, pc.podName)
			if err != nil {
				glog.Errorf("error starting unit %s : %v", unit.Name, err)
				pc.syncErrors[unit.Name] = *unitStatus
				pc.waitGroup.Done()
				return
			}
			delete(pc.syncErrors, unit.Name)
		}
		pc.waitGroup.Done()
	}()
	spec.Phase = api.PodRunning
	return event
}

func (pc *PodController) waitForInitUnit(ctx context.Context, name string, policy api.RestartPolicy) bool {
	for {
		select {
		case <-ctx.Done():
			glog.Infof("Cancelled waiting for init unit %s", name)
			return false
		case <-time.After(waitForInitUnitPollInterval):
			glog.Infof("Checking status of init unit %s", name)
		}
		u, err := itzounit.OpenUnit(pc.rootdir, name)
		if err != nil {
			glog.Warningf("Opening init unit %s: %v", name, err)
			continue
		}
		status, err := u.GetStatus()
		if err != nil {
			glog.Warningf("Getting status of init unit %s: %v", name, err)
			continue
		}
		glog.Infof("Init unit %s status is %+v", name, status)
		if status.State.Terminated != nil {
			ec := status.State.Terminated.ExitCode
			glog.Infof("Init unit %s exited with %d", name, ec)
			// If the init unit succeded, return true otherwise, keep
			// going unless hte restart policy is never, in that case
			// we return false.
			if ec == 0 {
				return true
			} else if policy == api.RestartPolicyNever {
				return false
			}
		}
	}
}

func (pc *PodController) GetStatus() ([]api.UnitStatus, []api.UnitStatus, error) {
	var statuses []api.UnitStatus
	var initStatuses []api.UnitStatus
	for _, unit := range pc.podStatus.Units {
		unitStatus, err := pc.runtime.ContainerStatus(unit.Name, unit.Image)
		if err != nil {
			glog.Error(err)
		}
		failedStatus, exists := pc.syncErrors[unit.Name]
		if exists {
			unitStatus = &failedStatus
		}
		statuses = append(statuses, *unitStatus)
	}
	for _, unit := range pc.podStatus.InitUnits {
		unitStatus, err := pc.runtime.ContainerStatus(unit.Name, unit.Image)
		if err != nil {
			glog.Error(err)
		}
		failedStatus, exists := pc.syncErrors[unit.Name]
		if exists {
			unitStatus = &failedStatus
		}
		initStatuses = append(initStatuses, *unitStatus)
	}

	// Kubelet reports completed init units as "Ready" whereas
	// completed regular units are not ready. Let's handle that
	// special case here
	for i := range initStatuses {
		if initStatuses[i].State.Terminated != nil &&
			initStatuses[i].State.Terminated.ExitCode == int32(0) {
			initStatuses[i].Ready = true
		}
	}
	return statuses, initStatuses, nil
}

// returns: units to start, init units to start
func (pc *PodController) RestartPod(spec, status *api.PodSpec) error {
	glog.Info("init units not equal, trying to restart pod")
	for _, unit := range status.Units {
		err := pc.runtime.RemoveContainer(&unit)
		if err != nil {
			return err
		}
	}
	err := pc.runtime.StopPodSandbox(status)
	if err != nil {
		return err
	}
	err = pc.runtime.RemovePodSandbox(status)
	if err != nil {
		return err
	}
	err = pc.runtime.RunPodSandbox(spec)
	if err != nil {
		return err
	}
	for _, unit := range spec.Units {
		unitStatus, err := pc.runtime.CreateContainer(unit, spec, pc.podName, pc.allCreds)
		if err != nil {
			pc.syncErrors[unit.Name] = *unitStatus
			return err
		}
	}
	pc.podRestartCount += 1
	spec.Phase = api.PodDispatching
	return nil
}

func (pc *PodController) CreatePod(spec *api.PodSpec) error {
	glog.Info("status units are nil, trying to create pod from scratch")
	err := pc.runtime.RunPodSandbox(spec)
	if err != nil {
		return err
	}
	for _, unit := range spec.Units {
		unitStatus, err := pc.runtime.CreateContainer(unit, spec, pc.podName, pc.allCreds)
		if err != nil {
			pc.syncErrors[unit.Name] = *unitStatus
			return err
		}
	}
	return nil
}

func (pc *PodController) RestartUnits(spec, status *api.PodSpec) ([]api.Unit, error) {
	addUnits, deleteUnits := diffUnits(spec.Units, status.Units)
	spec.Phase = api.PodWaiting
	for _, unit := range deleteUnits {
		err := pc.runtime.RemoveContainer(&unit)
		if err != nil {
			return []api.Unit{}, err
		}
	}

	for _, unit := range addUnits {
		unitStatus, err := pc.runtime.CreateContainer(unit, spec, pc.podName, pc.allCreds)
		if err != nil {
			pc.syncErrors[unit.Name] = *unitStatus
			return []api.Unit{}, err
		}
	}
	return addUnits, nil

}

// TODO
func (pc *PodController) GetLogBuffer(unitName string) (*logbuf.LogBuffer, error) {
	return pc.runtime.GetLogBuffer(unitName)
}

func (pc *PodController) ReadLogBuffer(unit string, n int) ([]logbuf.LogEntry, error) {
	return pc.runtime.ReadLogBuffer(unit, n)
}

func (pc *PodController) UnitRunning(unit string) bool {
	return pc.runtime.UnitRunning(unit)
}

func (pc *PodController) GetPid(unitName string) (int, bool) {
	return pc.runtime.GetPid(unitName)
}