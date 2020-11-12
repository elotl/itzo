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
	"github.com/containers/libpod/v2/pkg/bindings/containers"
	"github.com/containers/libpod/v2/pkg/bindings/play"
	"github.com/containers/libpod/v2/pkg/bindings/pods"
	"github.com/containers/libpod/v2/pkg/domain/entities"
	"github.com/ghodss/yaml"
	"github.com/instrumenta/kubeval/kubeval"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elotl/itzo/pkg/api"
	itzounit "github.com/elotl/itzo/pkg/unit"
	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/util/intstr"
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
}

// I know how to do one thing: Make Controllers. A ton of controllers...
type PodController struct {
	rootdir     string
	podName     string
	podHostname string
	mountCtl    Mounter
	unitMgr     UnitRunner
	imagePuller Puller
	podStatus   *api.PodSpec
	updateChan  chan *api.PodParameters
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

func NewPodController(ctx context.Context, rootdir string, mounter Mounter, unitMgr UnitRunner, usePodman bool) *PodController {
	var imgPuller Puller
	if usePodman {
		imgPuller = NewPodmanPuller(ctx)
		glog.Info("using podman image puller")
	} else {
		imgPuller = &ImagePuller{}
	}
	return &PodController{
		rootdir:     rootdir,
		unitMgr:     unitMgr,
		mountCtl:    mounter,
		imagePuller: imgPuller,
		updateChan:  make(chan *api.PodParameters, specChanSize),
		syncErrors:  make(map[string]api.UnitStatus),
		podStatus: &api.PodSpec{
			Phase:         api.PodRunning,
			RestartPolicy: api.RestartPolicyAlways,
		},
		cancelFunc: nil,
		podRestartCount: 0,
		usePodman: usePodman,
	}
}

func (pc *PodController) SetPodNetwork(netNS, podIP string) {
	pc.netNS = netNS
	pc.podIP = podIP
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

func makeFailedUpdateStatus(unit *api.Unit, msg string) api.UnitStatus {
	return api.UnitStatus{
		Name: unit.Name,
		State: api.UnitState{
			Waiting: &api.UnitStateWaiting{
				Reason:       msg,
				StartFailure: true,
			},
		},
		Image: unit.Image,
	}
}

func (pc *PodController) destroyUnit(unit *api.Unit) error {
	unitName := unit.Name
	glog.Infoln("Stopping unit", unitName)
	//
	// There's a few things here that need to happen in order:
	//   * Stop the unit (kill its main process).
	//   * Detach all its mounts.
	//   * Remove its files/directories.
	//
	err := pc.unitMgr.StopUnit(unitName)
	if err != nil {
		glog.Errorf("Error stopping unit %s: %v; trying to continue",
			unitName, err)
	}
	for _, mount := range unit.VolumeMounts {
		err = pc.mountCtl.DetachMount(unitName, mount.MountPath)
		if err != nil {
			glog.Errorf(
				"Error detaching mount %s from %s: %v; trying to continue",
				mount.Name, unitName, err)
		}
	}
	err = pc.unitMgr.RemoveUnit(unitName)
	if err != nil {
		glog.Errorf("Error removing unit %s; trying to continue",
			unitName)
	}
	return err
}

func (pc *PodController) DestroyPod(spec *api.PodSpec) {
	for _, unit := range append(spec.InitUnits, spec.Units...) {
		err := pc.destroyUnit(&unit)
		if err != nil {
			glog.Errorf("error while destroying unit: %s: %v", unit.Name, err)
		}
	}
	for _, volume := range spec.Volumes {
		err := pc.mountCtl.DeleteMount(&volume)
		if err != nil {
			glog.Errorf("Error removing volume %s: %v", volume.Name, err)
		}
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
	//fmt.Printf("%#v\n", *spec)
	//fmt.Printf("%#v\n", *status)
	glog.Info("syncing pod units...")
	event := detectChangeType(spec, status)
	glog.Infof("detected change: %s", event)
	var initsToStart []api.Unit
	var unitsToStart []api.Unit
	switch event {
	case UpdateTypeNoChanges:
		// there aren't any units to restart
		return event
	case UpdateTypeUnitsChange:
		addUnits, deleteUnits := diffUnits(spec.Units, status.Units)
		removeVolumes := false
		forceRemove := true
		if pc.usePodman {
			connText, err := itzounit.GetPodmanConnection()
			for _, unit := range deleteUnits {
				containerName := api.UnitNameToContainerName(unit.Name)
				err = containers.Stop(connText, containerName, nil)
				if err != nil {
					glog.Errorf("error stopping %s container: %v", containerName, err)
				}

				err = containers.Remove(connText, containerName, &forceRemove, &removeVolumes)
				if err != nil {
					glog.Errorf("error removing %s container: %v", containerName, err)
				}
			}
			for _, unit := range addUnits {
				containerName := api.UnitNameToContainerName(unit.Name)
				err = containers.Start(connText, containerName, nil)
				if err != nil {
					glog.Errorf("error starting %s container: %v", containerName, err)
				}
			}
			return event

			// podman stop unit-name && podman rm unit-name
			// podman run unit-name --pod pod-name
			// exit function
		}
		// do deletes
		for _, unit := range deleteUnits {
			// there are some units to delete
			err := pc.destroyUnit(&unit)
			if err != nil {
				makeFailedUpdateStatus(&unit, err.Error())
				glog.Errorf("Error during unit: %s destroy: %v", unit.Name, err)
			}
		}
		spec.Phase = api.PodWaiting
		initsToStart, unitsToStart = []api.Unit{}, addUnits
	case UpdateTypePodCreate:
		// start pod
		if pc.usePodman {
			connText, err := itzounit.GetPodmanConnection()
			// debug, todo, remove this log line
			glog.Infof("got kip pod spec: %v", spec)
			k8sPodSpec := api.PodSpecToK8sPodSpec(*spec)
			podYaml := api.K8sPodToYamlFormat(k8sPodSpec)
			tmpFile, err := ioutil.TempFile(os.TempDir(), "pod-*.yaml")
			if err != nil {
				fmt.Println("Cannot create temporary file", err)
			}

			// cleaning up by removing the file
			defer os.Remove(tmpFile.Name())

			// Write to the file
			fileContents, err := yaml.Marshal(podYaml)
			glog.Infof("file contents:\n %s", string(fileContents))
			_, err = kubeval.Validate(fileContents, kubeval.NewDefaultConfig())
			if err != nil {
				glog.Errorf("validation of k8s file failed with: %v", err)
			}
			if _, err = tmpFile.Write(fileContents); err != nil {
				glog.Errorf("Failed to write to temporary file: %v", err)
			}
			spec.Phase = api.PodWaiting
			report, err := play.Kube(connText, tmpFile.Name(), entities.PlayKubeOptions{})
			if err != nil {
				glog.Errorf("podman pod creating failed with: %v", err)
				spec.Phase = api.PodFailed
				return event
			}
			glog.Infof("created pod: %s", report)
			spec.Phase = api.PodRunning
			return event
			// podman play kube /tmp/<file created from spec>
			// save pod and container(s) ids
			// exit function
			// podmanManager := pc.unitMgr.(itzounit.PodmanManager)

			// play.Kube(pc.unitMgr.GetContext())
		}
		glog.Info("status units are nil, trying to create pod from scratch")
		for _, volume := range spec.Volumes {
			err := pc.mountCtl.CreateMount(&volume)
			if err != nil {
				glog.Errorf("Error creating volume: %s, %v", volume.Name, err)
			}
		}
		spec.Phase = api.PodDispatching
		initsToStart, unitsToStart = spec.InitUnits, spec.Units
	case UpdateTypePodRestart:
		if pc.usePodman {
			// delete pod
			// create pod from spec.
			// exit function
			connText, _ := itzounit.GetPodmanConnection()
			stopReport, err := pods.Stop(connText, "my-pod", nil)
			if err != nil || len(stopReport.Errs) > 0 {
				glog.Errorf("error stopping pod: %v", err)
				for _, err := range stopReport.Errs {
					glog.Errorf("errors stopping pod: %s", err.Error())
				}
			}
			removeReport, err := pods.Remove(connText, "my-pod", nil)
			if err != nil || removeReport.Err != nil {
				glog.Errorf("error stopping pod: %v \n%v", err, removeReport.Err)

			}
			// debug, todo, remove this log line
			glog.Infof("got kip pod spec: %v", spec)
			k8sPodSpec := api.PodSpecToK8sPodSpec(*spec)
			podYaml := api.K8sPodToYamlFormat(k8sPodSpec)
			tmpFile, err := ioutil.TempFile(os.TempDir(), "pod-*.yaml")
			if err != nil {
				fmt.Println("Cannot create temporary file", err)
			}

			// cleaning up by removing the file
			defer os.Remove(tmpFile.Name())

			// Write to the file
			fileContents, err := yaml.Marshal(podYaml)
			glog.Infof("file contents:\n %s", string(fileContents))
			_, err = kubeval.Validate(fileContents, kubeval.NewDefaultConfig())
			if err != nil {
				glog.Errorf("validation of k8s file failed with: %v", err)
			}
			if _, err = tmpFile.Write(fileContents); err != nil {
				glog.Errorf("Failed to write to temporary file: %v", err)
			}
			spec.Phase = api.PodWaiting
			report, err := play.Kube(connText, tmpFile.Name(), entities.PlayKubeOptions{})
			if err != nil {
				glog.Errorf("podman pod creating failed with: %v", err)
				spec.Phase = api.PodFailed
				return event
			}
			glog.Infof("created pod: %s", report)
			spec.Phase = api.PodRunning
			return event
		}
		glog.Info("init units not equal, trying to restart pod")
		pc.DestroyPod(status)
		for _, volume := range spec.Volumes {
			err := pc.mountCtl.CreateMount(&volume)
			if err != nil {
				glog.Errorf("Error creating volume: %s, %v", volume.Name, err)
			}
		}
		// if there is a change in any of init containers, we have to restart whole pod
		initsToStart, unitsToStart = spec.InitUnits, spec.Units
		spec.Phase = api.PodDispatching
		pc.podRestartCount += 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	if pc.cancelFunc != nil {
		glog.Infof("Canceling previous pod update")
		pc.cancelFunc()
	}
	pc.cancelFunc = cancel
	pc.waitGroup.Wait() // Wait for previous update to finish.
	pc.waitGroup = sync.WaitGroup{}
	pc.waitGroup.Add(1)
	go func() {
		pc.startAllUnits(ctx, allCreds, initsToStart, unitsToStart, spec.RestartPolicy, spec.SecurityContext)
		pc.waitGroup.Done()
	}()
	spec.Phase = api.PodRunning
	return event
}

func (pc *PodController) saveUnitConfig(unit *api.Unit, podSecurityContext *api.PodSecurityContext) error {
	unitConfig := itzounit.UnitConfig{
		StartupProbe:             translateProbePorts(unit, unit.StartupProbe),
		ReadinessProbe:           translateProbePorts(unit, unit.ReadinessProbe),
		LivenessProbe:            translateProbePorts(unit, unit.LivenessProbe),
		TerminationMessagePolicy: unit.TerminationMessagePolicy,
		TerminationMessagePath:   unit.TerminationMessagePath,
		PodIP:                    pc.podIP,
	}
	if podSecurityContext != nil {
		unitConfig.PodSecurityContext = *podSecurityContext
	}
	if unit.SecurityContext != nil {
		unitConfig.SecurityContext = *unit.SecurityContext
	}
	u, err := itzounit.OpenUnit(pc.rootdir, unit.Name)
	if err != nil {
		return fmt.Errorf("opening unit %q for saving unit configuration: %v",
			unit.Name, err)
	}
	err = u.SaveUnitConfig(unitConfig)
	if err != nil {
		return fmt.Errorf("saving unit %q configuration: %v",
			unit.Name, err)
	}
	return nil
}

// Dockerhub can go by several names that the user can specify in
// their image spec.  .docker/config.json uses index.docker.io but an
// empty server should also map to that. We have used
// registry-1.docker.io internally and users might also just say
// "docker.io".  Try to find the server that shows up in the
// credentials sent over from kip.
func getRepoCreds(server string, allCreds map[string]api.RegistryCredentials) (string, string) {
	if creds, ok := allCreds[server]; ok {
		return creds.Username, creds.Password
	}
	// if credentials weren't found and the server is dockerhub
	// (server is empty or ends in docker.io), try to find credentials
	// for (in this order): index.docker.io, registry-1.docker.io,
	// docker.io
	if server == "" || strings.HasSuffix(server, "docker.io") {
		possibleServers := []string{"index.docker.io", "registry-1.docker.io", "docker.io"}
		for _, possibleServer := range possibleServers {
			if creds, ok := allCreds[possibleServer]; ok {
				return creds.Username, creds.Password
			}
		}
	}
	return "", ""
}

func (pc *PodController) startUnit(ctx context.Context, unit api.Unit, allCreds map[string]api.RegistryCredentials, policy api.RestartPolicy, podSecurityContext *api.PodSecurityContext) {
	glog.Infof("pc.startUnit, using podman: %s", strconv.FormatBool(pc.usePodman))
	if !pc.usePodman {

		// pull image
		server, imageRepo, err := util.ParseImageSpec(unit.Image)
		if err != nil {
			msg := fmt.Sprintf("Bad image spec for unit %s: %v", unit.Name, err)
			pc.syncErrors[unit.Name] = makeFailedUpdateStatus(&unit, msg)
			return
		}
		username, password := getRepoCreds(server, allCreds)
		err = pc.imagePuller.PullImage(pc.rootdir, unit.Name, imageRepo, server, username, password)
		if err != nil {
			msg := fmt.Sprintf("Error pulling image for unit %s: %v",
				unit.Name, err)
			pc.syncErrors[unit.Name] = makeFailedUpdateStatus(&unit, msg)
			return
		}

		err = pc.saveUnitConfig(&unit, podSecurityContext)
		if err != nil {
			msg := fmt.Sprintf("Error saving unit %s configuration: %v",
				unit.Name, err)
			pc.syncErrors[unit.Name] = makeFailedUpdateStatus(&unit, msg)
			return
		}
		// attach mounts
		mountFailure := false
		for _, mount := range unit.VolumeMounts {
			err := pc.mountCtl.AttachMount(
				unit.Name, mount.Name, mount.MountPath)
			if err != nil {
				msg := fmt.Sprintf("Error attaching mount %s to unit %s: %v",
					mount.Name, unit.Name, err)
				pc.syncErrors[unit.Name] = makeFailedUpdateStatus(&unit, msg)
				mountFailure = true
				break
			}
		}
		if mountFailure {
			return
		}
	}

	glog.Infoln("Starting unit", unit.Name)
	err := pc.unitMgr.StartUnit(
		pc.podName,
		pc.podHostname,
		unit.Name,
		unit.WorkingDir,
		pc.netNS,
		unit.Command,
		unit.Args,
		makeAppEnv(&unit),
		policy)
	if err != nil {
		msg := fmt.Sprintf("Error starting unit %s: %v",
			unit.Name, err)
		pc.syncErrors[unit.Name] = makeFailedUpdateStatus(&unit, msg)
		return
	}
	delete(pc.syncErrors, unit.Name)
}

// Our probes can reference unit ports by the name given to them in
// the unit.  However, where the probes are actually used (inside
// unit.go) we don't have access to the full unit structure. Here we
// make a copy of an httpget probe and update it to use only port
// numbers. If we can't look up a probe's name, we don't fail the
// unit, instead we pass along the unmodified port and it'll fail in
// the probe. That matches the behavior of the kubelet.  Note that we
// can't change a probe in-place since that messes up the diffs we do
// on pods during UpdatePod and would cause us to delete and recreate
// pods.
func translateProbePorts(unit *api.Unit, probe *api.Probe) *api.Probe {
	// only translate the port name if this is an http probe with a port
	// with a string name
	if probe != nil &&
		probe.HTTPGet != nil &&
		probe.HTTPGet.Port.Type == intstr.String {
		p := *probe
		port, err := findPortByName(unit, p.HTTPGet.Port.StrVal)
		if err != nil {
			glog.Errorf("error looking up probe port: %s", err)
		} else {
			newAction := *p.HTTPGet
			p.HTTPGet = &newAction
			p.HTTPGet.Port = intstr.FromInt(port)
		}
		return &p
	} else {
		return probe
	}
}

// findPortByName is a helper function to look up a port in a container by name.
func findPortByName(unit *api.Unit, portName string) (int, error) {
	for _, port := range unit.Ports {
		if port.Name == portName {
			return int(port.ContainerPort), nil
		}
	}
	return 0, fmt.Errorf("port %s not found", portName)
}

func (pc *PodController) startAllUnits(ctx context.Context, allCreds map[string]api.RegistryCredentials, initUnits []api.Unit, addUnits []api.Unit, policy api.RestartPolicy, podSecurityContext *api.PodSecurityContext) {
	ipolicy := policy
	if ipolicy == api.RestartPolicyAlways {
		// Restart policy "Always" is nonsensical for init units.
		ipolicy = api.RestartPolicyOnFailure
	}
	for _, unit := range initUnits {
		// Start init units first, one by one, and wait for each to finish.
		pc.startUnit(ctx, unit, allCreds, ipolicy, podSecurityContext)
		if !pc.waitForInitUnit(ctx, unit.Name, ipolicy) {
			return
		}
	}
	for _, unit := range addUnits {
		pc.startUnit(ctx, unit, allCreds, policy, podSecurityContext)
	}

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

func makeAppEnv(unit *api.Unit) []string {
	e := []string{}
	for _, ev := range unit.Env {
		e = append(e, fmt.Sprintf("%s=%s", ev.Name, ev.Value))
	}
	return e
}

type ImagePuller struct {
}

func (ip *ImagePuller) PullImage(rootdir, name, image, server, username, password string) error {
	if server == "docker.io" {
		// K8s and Helm might set this for images, but the actual official
		// registry is registry-1.docker.io.
		server = "registry-1.docker.io"
	}
	if server != "" && !strings.HasPrefix(server, "http") {
		server = "https://" + server
	}
	glog.Infof("Creating new unit '%s' in %s\n", name, rootdir)
	u, err := itzounit.OpenUnit(rootdir, name)
	if err != nil {
		return fmt.Errorf("opening unit %s for package deploy: %v", name, err)
	}
	err = u.PullAndExtractImage(image, server, username, password)
	if err != nil {
		return fmt.Errorf("pulling image %s: %v", image, err)
	}
	return nil
}

func (pc *PodController) getUnitStatuses(units []api.Unit) []api.UnitStatus {
	// go through listed units in the spec, get their status
	// go through syncErrors, merge those in
	unitStatusMap := make(map[string]*api.UnitStatus)
	for _, podUnit := range units {
		// when errors opening the
		if !itzounit.IsUnitExist(pc.rootdir, podUnit.Name) {
			reason := "PodInitializing"
			unitStatusMap[podUnit.Name] = api.MakeStillCreatingStatus(
				podUnit.Name, podUnit.Image, reason)
			continue
		}
		unit, err := itzounit.OpenUnit(pc.rootdir, podUnit.Name)
		if err != nil {
			reason := "PodInitializing"
			unitStatusMap[podUnit.Name] = api.MakeStillCreatingStatus(
				podUnit.Name, podUnit.Image, reason)
			continue
		}
		us, err := unit.GetStatus()
		if err != nil {
			reason := "PodInitializing"
			unitStatusMap[podUnit.Name] = api.MakeStillCreatingStatus(
				podUnit.Name, podUnit.Image, reason)
			continue
		}
		unitStatusMap[podUnit.Name] = us
		syncFailStatus, exists := pc.syncErrors[podUnit.Name]
		if exists {
			unitStatusMap[syncFailStatus.Name] = &syncFailStatus
		}
	}
	unitStatuses := make([]api.UnitStatus, 0, len(unitStatusMap))
	for _, s := range unitStatusMap {
		unitStatuses = append(unitStatuses, *s)
	}
	return unitStatuses
}

func (pc *PodController) GetStatus() ([]api.UnitStatus, []api.UnitStatus, error) {
	if pc.usePodman {
		statuses, initStatuses, err := pc.getContainerStatuses()
		if err != nil {
			glog.Errorf("error getting container status from podman: %v", err)
		}
		return statuses, initStatuses, nil
	}
	statuses := pc.getUnitStatuses(pc.podStatus.Units)
	initStatuses := pc.getUnitStatuses(pc.podStatus.InitUnits)
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

func (pc *PodController) getContainerStatuses() ([]api.UnitStatus, []api.UnitStatus, error) {
	connText, err := itzounit.GetPodmanConnection()
	if err != nil {
		glog.Errorf("cannot get podman conn: %v", err)
	}
	var unitStatuses []api.UnitStatus
	for _, unit := range pc.podStatus.Units {
		containerName := api.UnitNameToContainerName(unit.Name)
		ctrData, err := containers.Inspect(connText, containerName, nil)
		if err != nil || ctrData == nil {
			// todo handle
			glog.Warningf("cannot get container state from podman for unit: %s\n%v \nctrData: %v", containerName, err, ctrData)
			continue
		}
		state, ready := ContainerStateToUnit(*ctrData)
		unitStatuses = append(unitStatuses, api.UnitStatus{
			Name:         unit.Name,
			State:        state,
			RestartCount: 0,
			Image:        unit.Image,
			Ready:        ready,
		})
	}
	return unitStatuses, []api.UnitStatus{}, nil
}