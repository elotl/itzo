package server

import (
	"fmt"
	"strings"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/util"
	"github.com/elotl/itzo/pkg/util/sets"
	"github.com/golang/glog"
)

var specChanSize = 100

type Puller interface {
	PullImage(rootdir, name, image, server, username, password string) error
}

type Mounter interface {
	CreateMount(*api.Volume) error
	DeleteMount(*api.Volume) error
	AttachMount(unitname, src, dst string) error
}

// Too bad there isn't a word for a creator AND destroyer
// Coulda gone with Shiva(er) but that's a bit imprecise...
type UnitRunner interface {
	StartUnit(string, []string, []string, []string, api.RestartPolicy) error
	StopUnit(string) error
}

// I know how to do one thing: Make Controllers. A fuckload of controllers...
type PodController struct {
	rootdir     string
	mountCtl    Mounter
	unitMgr     UnitRunner
	imagePuller Puller
	podStatus   *api.PodSpec
	updateChan  chan *api.PodParameters
	syncErrors  map[string]api.UnitStatus
}

func NewPodController(rootdir string, mounter Mounter, unitMgr UnitRunner) *PodController {
	return &PodController{
		rootdir:     rootdir,
		unitMgr:     unitMgr,
		mountCtl:    mounter,
		imagePuller: &ImagePuller{},
		updateChan:  make(chan *api.PodParameters, specChanSize),
		syncErrors:  make(map[string]api.UnitStatus),
		podStatus: &api.PodSpec{
			Phase:         api.PodRunning,
			RestartPolicy: api.RestartPolicyAlways,
		},
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
		spec := &podParams.Spec
		MergeSecretsIntoSpec(podParams.Secrets, spec)
		pc.syncErrors = pc.SyncPodUnits(spec, pc.podStatus, podParams.Credentials)
		pc.podStatus = spec
	}
}

func (pc *PodController) Start() {
	go pc.runUpdateLoop()
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
func MergeSecretsIntoSpec(secrets map[string]map[string][]byte, spec *api.PodSpec) {
	for i := 0; i < len(spec.Units); i++ {
		newEnv := make([]api.EnvVar, 0, len(spec.Units[i].Env))
		for _, ev := range spec.Units[i].Env {
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
		spec.Units[i].Env = newEnv
	}
}

func makeFailedUpdateStatus(unit *api.Unit, msg string) api.UnitStatus {
	return api.UnitStatus{
		Name: unit.Name,
		State: api.UnitState{
			Waiting: &api.UnitStateWaiting{
				Reason:        msg,
				LaunchFailure: true,
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

func DiffUnits(spec []api.Unit, status []api.Unit, allModifiedVolumes sets.String) (map[string]api.Unit, map[string]api.Unit) {
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

	return addUnits, deleteUnits
}

func (pc *PodController) SyncPodUnits(spec *api.PodSpec, status *api.PodSpec, allCreds map[string]api.RegistryCredentials) map[string]api.UnitStatus {
	// By this point, spec must have had the secrets merged into the env vars
	//fmt.Printf("%#v\n", *spec)
	//fmt.Printf("%#v\n", *status)
	addVolumes, deleteVolumes, allModifiedVolumes := DiffVolumes(spec.Volumes, status.Volumes)
	addUnits, deleteUnits := DiffUnits(spec.Units, status.Units, allModifiedVolumes)

	// do deletes
	for unitName, _ := range deleteUnits {
		glog.Infoln("Stopping unit", unitName)
		err := pc.unitMgr.StopUnit(unitName)
		if err != nil {
			glog.Errorf("Error deleting unit %s, trying to continue", unitName)
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

	// if a delete fails, attempt to carry on.  If an add fails,
	// collect the units that failed and pass them back to the caller
	// so that we can update the unit's status and bubble up the data
	// to milpa.
	erroredUnits := make(map[string]api.UnitStatus)
	for _, unit := range addUnits {
		// pull image
		server, imageRepo, err := util.ParseImageSpec(unit.Image)
		if err != nil {
			msg := fmt.Sprintf("Bad image spec for unit %s: %v", unit.Name, err)
			erroredUnits[unit.Name] = makeFailedUpdateStatus(&unit, msg)
			continue
		}
		creds := allCreds[server]
		err = pc.imagePuller.PullImage(pc.rootdir, unit.Name, imageRepo, server, creds.Username, creds.Password)
		if err != nil {
			msg := fmt.Sprintf("Error pulling image for unit %s: %v",
				unit.Name, err)
			erroredUnits[unit.Name] = makeFailedUpdateStatus(&unit, msg)
			continue
		}

		// attach mounts
		mountFailure := false
		for _, mount := range unit.VolumeMounts {
			err := pc.mountCtl.AttachMount(
				unit.Name, mount.Name, mount.MountPath)
			if err != nil {
				msg := fmt.Sprintf("Error attaching mount %s to unit %s: %v",
					mount.Name, unit.Name, err)
				erroredUnits[unit.Name] = makeFailedUpdateStatus(&unit, msg)
				mountFailure = true
				break
			}
		}
		if mountFailure {
			continue
		}

		glog.Infoln("Starting unit", unit.Name)
		err = pc.unitMgr.StartUnit(
			unit.Name, unit.Command, unit.Args, makeAppEnv(&unit), spec.RestartPolicy)
		if err != nil {
			msg := fmt.Sprintf("Error starting unit %s: %v",
				unit.Name, err)
			erroredUnits[unit.Name] = makeFailedUpdateStatus(&unit, msg)
			continue
		}
	}
	return erroredUnits
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
	if server != "" && !strings.HasPrefix(server, "http") {
		server = "https://" + server
	}
	glog.Infof("Creating new unit '%s' in %s\n", name, rootdir)
	u, err := OpenUnit(rootdir, name)
	if err != nil {
		return fmt.Errorf("opening unit %s for package deploy: %v", name, err)
	}
	defer u.Close()
	err = u.PullAndExtractImage(image, server, username, password)
	if err != nil {
		return fmt.Errorf("pulling image %s: %v", image, err)
	}
	return nil
}

func (pc *PodController) GetStatus() ([]api.UnitStatus, error) {
	// go through listed units in the spec, get their status
	// go through syncErrors, merge those in
	unitStatusMap := make(map[string]*api.UnitStatus)
	for _, podUnit := range pc.podStatus.Units {
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
	}
	for _, syncFailStatus := range pc.syncErrors {
		unitStatusMap[syncFailStatus.Name] = &syncFailStatus
	}
	unitStatuses := make([]api.UnitStatus, 0, len(unitStatusMap))
	for _, s := range unitStatusMap {
		unitStatuses = append(unitStatuses, *s)
	}
	return unitStatuses, nil
}