package server

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/util"
	"github.com/elotl/itzo/pkg/util/sets"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

var specChanSize = 100

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
	StartUnit(string, string, string, []string, []string, []string, api.RestartPolicy) error
	StopUnit(string) error
	RemoveUnit(string) error
}

// I know how to do one thing: Make Controllers. A fuckload of controllers...
type PodController struct {
	rootdir           string
	podName           string
	mountCtl          Mounter
	unitMgr           UnitRunner
	imagePuller       Puller
	resolvConfUpdater ResolvConfUpdater
	podStatus         *api.PodSpec
	updateChan        chan *api.PodParameters
	// We keep syncErrors in the map between syncs until a sync works
	// and we clear or overwrite the error
	syncErrors map[string]api.UnitStatus
	cancelFunc context.CancelFunc
	waitGroup  sync.WaitGroup
}

func NewPodController(rootdir string, mounter Mounter, unitMgr UnitRunner, resolvConfUpdater ResolvConfUpdater) *PodController {
	return &PodController{
		rootdir:           rootdir,
		unitMgr:           unitMgr,
		mountCtl:          mounter,
		imagePuller:       &ImagePuller{},
		resolvConfUpdater: resolvConfUpdater,
		updateChan:        make(chan *api.PodParameters, specChanSize),
		syncErrors:        make(map[string]api.UnitStatus),
		podStatus: &api.PodSpec{
			Phase:         api.PodRunning,
			RestartPolicy: api.RestartPolicyAlways,
		},
		cancelFunc: nil,
	}
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
		spec := &podParams.Spec
		MergeSecretsIntoSpec(podParams.Secrets, spec.Units)
		MergeSecretsIntoSpec(podParams.Secrets, spec.InitUnits)
		err := pc.resolvConfUpdater.UpdateSearch(podParams.ClusterName, podParams.Namespace)
		if err != nil {
			glog.Errorln("Error updating resolv.conf with cluster parameters", err)
		}
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

// Only diff the parts of the unit we care about
type MiniUnit struct {
	Name         string
	Image        string
	Command      []string
	Args         []string
	VolumeMounts []api.VolumeMount
	Env          []api.EnvVar
}

func makeMiniUnit(u *api.Unit) MiniUnit {
	return MiniUnit{
		Name:         u.Name,
		Image:        u.Image,
		Command:      u.Command,
		Args:         u.Args,
		VolumeMounts: u.VolumeMounts,
	}
}

func unitToMiniUnitMap(units []api.Unit) map[string]interface{} {
	m := make(map[string]interface{})
	for _, u := range units {
		m[u.Name] = makeMiniUnit(&u)
	}
	return m
}

func unitToUnitMap(units []api.Unit) map[string]api.Unit {
	m := make(map[string]api.Unit)
	for _, u := range units {
		m[u.Name] = u
	}
	return m
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

func volumesToMap(volumes []api.Volume) map[string]interface{} {
	m := make(map[string]interface{})
	for _, v := range volumes {
		m[v.Name] = v
	}
	return m
}

func DiffVolumes(spec []api.Volume, status []api.Volume) (map[string]api.Volume, map[string]api.Volume, sets.String) {
	allModifiedVolumes := sets.NewString()

	specMap := volumesToMap(spec)
	statusMap := volumesToMap(status)
	add, update, delete := util.MapDiff(specMap, statusMap)

	// Updates need to be deleted and then added
	delete = append(delete, update...)
	add = append(add, update...)

	addVolumes := make(map[string]api.Volume)
	for _, volName := range add {
		addVolumes[volName] = specMap[volName].(api.Volume)
		allModifiedVolumes.Insert(volName)
	}
	deleteVolumes := make(map[string]api.Volume)
	for _, volName := range delete {
		deleteVolumes[volName] = statusMap[volName].(api.Volume)
		allModifiedVolumes.Insert(volName)
	}
	return addVolumes, deleteVolumes, allModifiedVolumes
}

func DiffUnits(spec []api.Unit, status []api.Unit, allModifiedVolumes sets.String) ([]api.Unit, []api.Unit) {
	miniSpecMap := unitToMiniUnitMap(spec)
	specMap := unitToUnitMap(spec)
	miniStatusMap := unitToMiniUnitMap(status)
	statusMap := unitToUnitMap(status)

	add, update, delete := util.MapDiff(miniSpecMap, miniStatusMap)

	// Updates need to be deleted and then added
	delete = append(delete, update...)
	add = append(add, update...)

	addUnits := make(map[string]api.Unit)
	for _, unitName := range add {
		addUnits[unitName] = specMap[unitName]
	}
	deleteUnits := make(map[string]api.Unit)
	for _, unitName := range delete {
		deleteUnits[unitName] = statusMap[unitName]
	}

	// Go through all modified volume names, find any running units that
	// depend on those volumes.  Those units need to be deleted.
	// go through any speced units, if any of those depend on the
	// volumes, those need to be added
	for _, u := range status {
		for _, vol := range u.VolumeMounts {
			if allModifiedVolumes.Has(vol.Name) {
				deleteUnits[u.Name] = statusMap[u.Name]
			}
		}
	}
	for _, u := range spec {
		for _, vol := range u.VolumeMounts {
			if allModifiedVolumes.Has(vol.Name) {
				addUnits[u.Name] = specMap[u.Name]
			}
		}
	}

	// Order does matter for initunits, so let's convert the map back to a
	// list, preserving original order.
	addList := make([]api.Unit, 0)
	deleteList := make([]api.Unit, 0)
	for _, u := range spec {
		_, exists := addUnits[u.Name]
		if exists {
			addList = append(addList, u)
		}
	}
	for _, u := range status {
		_, exists := deleteUnits[u.Name]
		if exists {
			deleteList = append(deleteList, u)
		}
	}

	return addList, deleteList
}

func (pc *PodController) SyncPodUnits(spec *api.PodSpec, status *api.PodSpec, allCreds map[string]api.RegistryCredentials) {
	// By this point, spec must have had the secrets merged into the env vars
	//fmt.Printf("%#v\n", *spec)
	//fmt.Printf("%#v\n", *status)
	addVolumes, deleteVolumes, allModifiedVolumes := DiffVolumes(spec.Volumes, status.Volumes)
	addUnits, deleteUnits := DiffUnits(spec.Units, status.Units, allModifiedVolumes)
	// TODO: according to the K8s specs, if "A user updates the PodSpec causing
	// the Init Container image to change" then the entire pod is restarted.
	addInits, deleteInits := DiffUnits(spec.InitUnits, status.InitUnits, allModifiedVolumes)

	// do deletes
	for _, unit := range append(deleteUnits, deleteInits...) {
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
	}
	for _, volume := range deleteVolumes {
		err := pc.mountCtl.DeleteMount(&volume)
		if err != nil {
			glog.Errorf("Error removing volume %s: %v", volume.Name, err)
		}
	}
	// do adds
	for _, volume := range addVolumes {
		err := pc.mountCtl.CreateMount(&volume)
		if err != nil {
			glog.Errorf("Error creating volume: %s, %v", volume.Name, err)
		}
	}
	if len(addInits) > 0 || len(addUnits) > 0 {
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
			pc.startAllUnits(ctx, allCreds, addInits, addUnits, spec.RestartPolicy, spec.SecurityContext)
			pc.waitGroup.Done()
		}()
	}
}

func (pc *PodController) saveSecurityContext(unitName string, podSecurityContext *api.PodSecurityContext, unitSecurityContext *api.SecurityContext) error {
	glog.Infof("Saving security context for unit %q in %q", unitName, pc.rootdir)
	u, err := OpenUnit(pc.rootdir, unitName)
	if err != nil {
		return fmt.Errorf("Opening unit %q for saving security context: %v",
			unitName, err)
	}
	err = u.SaveSecurityContext(podSecurityContext, unitSecurityContext)
	if err != nil {
		return fmt.Errorf("Saving security context for unit %q: %v",
			unitName, err)
	}
	return nil
}

func (pc *PodController) startUnit(ctx context.Context, unit api.Unit, allCreds map[string]api.RegistryCredentials, policy api.RestartPolicy, podSecurityContext *api.PodSecurityContext) {
	// pull image
	server, imageRepo, err := util.ParseImageSpec(unit.Image)
	if err != nil {
		msg := fmt.Sprintf("Bad image spec for unit %s: %v", unit.Name, err)
		pc.syncErrors[unit.Name] = makeFailedUpdateStatus(&unit, msg)
		return
	}
	creds := allCreds[server]
	err = pc.imagePuller.PullImage(pc.rootdir, unit.Name, imageRepo, server, creds.Username, creds.Password)
	if err != nil {
		msg := fmt.Sprintf("Error pulling image for unit %s: %v",
			unit.Name, err)
		pc.syncErrors[unit.Name] = makeFailedUpdateStatus(&unit, msg)
		return
	}

	// Save security context.
	err = pc.saveSecurityContext(
		unit.Name,
		podSecurityContext,
		unit.SecurityContext)
	if err != nil {
		msg := fmt.Sprintf("Error saving security context for unit %s: %v",
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

	glog.Infoln("Starting unit", unit.Name)
	err = pc.unitMgr.StartUnit(
		pc.podName,
		unit.Name,
		unit.WorkingDir,
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

func (pc *PodController) startAllUnits(ctx context.Context, allCreds map[string]api.RegistryCredentials, initUnits []api.Unit, addUnits []api.Unit, policy api.RestartPolicy, podSecurityContext *api.PodSecurityContext) {
	ipolicy := policy
	if ipolicy == api.RestartPolicyAlways {
		// Restart policy "Always" is nonsensical for init units.
		ipolicy = api.RestartPolicyOnFailure
	}
	for _, unit := range initUnits {
		// Start init units first, one by one, and wait for each to finish.
		pc.startUnit(ctx, unit, allCreds, ipolicy, podSecurityContext)
		if !pc.waitForUnit(ctx, unit.Name) {
			return
		}
	}
	for _, unit := range addUnits {
		pc.startUnit(ctx, unit, allCreds, policy, podSecurityContext)
	}
}

func (pc *PodController) waitForUnit(ctx context.Context, name string) bool {
	for {
		select {
		case <-ctx.Done():
			glog.Infof("Cancelled waiting for init unit %s", name)
			return false
		case <-time.After(1 * time.Second):
			glog.Infof("Checking status of init unit %s", name)
		}
		u, err := OpenUnit(pc.rootdir, name)
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
			succeeded := false
			ec := status.State.Terminated.ExitCode
			if ec == 0 {
				succeeded = true
			}
			glog.Infof("Init unit %s exited with %d", name, ec)
			return succeeded
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
	u, err := OpenUnit(rootdir, name)
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
		if !IsUnitExist(pc.rootdir, podUnit.Name) {
			reason := "Unit waiting"
			unitStatusMap[podUnit.Name] = makeStillCreatingStatus(
				podUnit.Name, podUnit.Image, reason)
			continue
		}
		unit, err := OpenUnit(pc.rootdir, podUnit.Name)
		if err != nil {
			reason := "Constructing unit"
			unitStatusMap[podUnit.Name] = makeStillCreatingStatus(
				podUnit.Name, podUnit.Image, reason)
			continue
		}
		us, err := unit.GetStatus()
		if err != nil {
			reason := "Constructing unit, no status yet"
			unitStatusMap[podUnit.Name] = makeStillCreatingStatus(
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
	statuses := pc.getUnitStatuses(pc.podStatus.Units)
	initStatuses := pc.getUnitStatuses(pc.podStatus.InitUnits)
	return statuses, initStatuses, nil
}
