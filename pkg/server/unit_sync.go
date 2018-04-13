package server

import (
	"fmt"
	"strings"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/util"
	"github.com/elotl/itzo/pkg/util/sets"
	"github.com/golang/glog"
)

type Puller interface {
	PullImage(name, image, server, username, password string) error
}

type Mounter interface {
	CreateMount(string, *api.Volume) error
	DeleteMount(string, *api.Volume) error
	AttachMount(basedir, unitname, src, dst string) error
}

// Too bad there isn't a word for a creator AND destroyer
// Coulda gone with Shiva(er) but that's a bit imprecise...
type UnitManager interface {
	AddUnit(string, *api.Unit, []string, api.RestartPolicy) error
	DeleteUnit(string) error
}

type UnitSync struct {
	basedir     string
	mountCtl    Mounter
	unitCtl     UnitManager
	imagePuller Puller
}

// We need to remove the Ports from the unit spec since they
// aren't used on the nodes
type MiniUnit struct {
	Name         string
	Image        string
	Command      string
	VolumeMounts []api.VolumeMount
	Env          []api.EnvVar
}

func makeMiniUnit(u *api.Unit) MiniUnit {
	return MiniUnit{
		Name:         u.Name,
		Image:        u.Image,
		Command:      u.Command,
		VolumeMounts: u.VolumeMounts,
	}
}

func specToUnitMap(s *api.PodSpec) map[string]interface{} {
	m := make(map[string]interface{})
	for _, u := range s.Units {
		m[u.Name] = makeMiniUnit(&u)
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

func makeFailedStatus(unit *api.Unit, msg string) api.UnitStatus {
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

func DiffVolumes(spec *api.PodSpec, status *api.PodSpec) (map[string]api.Volume, map[string]api.Volume, sets.String) {
	allModifiedVolumes := sets.NewString()

	specMap := volumesToMap(spec.Volumes)
	statusMap := volumesToMap(status.Volumes)
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

func DiffUnits(spec *api.PodSpec, status *api.PodSpec, allModifiedVolumes sets.String) (map[string]api.Unit, map[string]api.Unit) {
	specMap := specToUnitMap(spec)
	statusMap := specToUnitMap(status)
	add, update, delete := util.MapDiff(specMap, statusMap)

	// Updates need to be deleted and then added
	delete = append(delete, update...)
	add = append(add, update...)

	addUnits := make(map[string]api.Unit)
	for _, unitName := range add {
		addUnits[unitName] = specMap[unitName].(api.Unit)
	}
	deleteUnits := make(map[string]api.Unit)
	for _, unitName := range delete {
		deleteUnits[unitName] = statusMap[unitName].(api.Unit)
	}

	// Go through all modified volume names, find any running units that
	// depend on those volumes.  Those units need to be deleted.
	// go through any speced units, if any of those depend on the
	// volumes, those need to be added
	for _, u := range status.Units {
		for _, vol := range u.VolumeMounts {
			if allModifiedVolumes.Has(vol.Name) {
				deleteUnits[u.Name] = statusMap[u.Name].(api.Unit)
			}
		}
	}
	for _, u := range spec.Units {
		for _, vol := range u.VolumeMounts {
			if allModifiedVolumes.Has(vol.Name) {
				addUnits[u.Name] = specMap[u.Name].(api.Unit)
			}
		}
	}

	return addUnits, deleteUnits
}

func (us *UnitSync) SyncPodUnits(spec *api.PodSpec, status *api.PodSpec) map[string]api.UnitStatus {
	// By this point, spec must have had the secrets merged into the env vars
	addVolumes, deleteVolumes, allModifiedVolumes := DiffVolumes(spec, status)
	addUnits, deleteUnits := DiffUnits(spec, status, allModifiedVolumes)

	// do deletes
	for unitName, _ := range deleteUnits {
		err := us.unitCtl.DeleteUnit(unitName)
		if err != nil {
			glog.Errorf("Error deleting unit %s, trying to continue", unitName)
		}
	}
	for _, volume := range deleteVolumes {
		err := us.mountCtl.DeleteMount(us.baseDir, volume)
		if err != nil {
			glog.Errorf("Error removing volume %s: %v", volume.Name, err)
		}
	}
	// do adds
	for _, volume := range addVolumes {
		err := us.mountCtl.CreateMount(us.baseDir, volume)
		if err != nil {
			glog.Errorf("Error creating volume: %s, %v", volume.Name, err)
		}
	}

	// if a delete fails, attempt to carry on.  If an add fails,
	// collect the units that failed and pass them back to the caller
	// so that we can update the unit's status and bubble up the data
	// to milpa.

	erroredUnits := make(map[string]api.UnitStatus)
	for unitName, unit := range addUnits {
		// pull image
		server, imageRepo, err := util.ParseImageSpec(unit.Image)
		if err != nil {
			msg := fmt.Sprintf("Bad image spec for unit %s: %v", unit.Name, err)
			erroredUnits[unit.Name] = makeFailedStatus(&unit, msg)
			continue
		}
		creds := allCreds[server]
		err = pullImage(unit.Name, image, server, creds.username, creds.password)
		if err != nil {
			msg := fmt.Sprintf("Error pulling image for unit %s: %v",
				unit.Name, err)
			erroredUnits[unit.Name] = makeFailedStatus(&unit, msg)
			continue
		}

		// attach mounts
		mountFailure := false
		for _, mount := range unit.VolumeMounts {
			err := us.mountCtl.AttachMount(
				us.baseDir, unit.Name, mount.Name, mount.MountPath)
			if err != nil {
				msg := fmt.Sprintf("Error attaching mount %s to unit %s: %v",
					mount.Name, unit.Name, err)
				erroredUnits[unit.Name] = makeFailedStatus(&unit, msg)
				mountFailure = true
				break
			}
		}
		if mountFailure {
			continue
		}

		err = us.unitCtl.AddUnit(
			us.baseDir, unit, makeAppEnv(unit),
			RestartPolicy(pod.Spec.RestartPolicy))
		// err = startUnitHelper(
		// 	rootDir, unit, unit.Command, makeAppEnv(unit),
		// 	RestartPolicy(pod.Spec.RestartPolicy))
		if err != nil {
			msg := fmt.Sprintf("Error starting unit %s: %v",
				unit.Name, err)
			erroredUnits[unit.Name] = makeFailedStatus(unit, msg)
			continue
		}
	}
	return erroredUnits
}

type ImagePuller struct {
}

func (ip *ImagePuller) PullImage(name, image, server, username, password string) error {
	installRootDir := "/tmp/itzo"
	if server != "" && !strings.HasPrefix(server, "http") {
		server = "https://" + server
	}
	glog.Infof("Creating new unit '%s' in %s\n", name, installRootDir)
	u, err := OpenUnit(installRootDir, name)
	if err != nil {
		return fmt.Errorf("opening unit %s for package deploy: %v", name, err)
	}
	defer u.Close()
	//rootfs := u.GetRootfs()

	err = u.PullAndExtractImage(image, server, username, password)
	if err != nil {
		return fmt.Errorf("pulling image %s: %v", image, err)
	}
}

func volumesToMap(volumes []api.Volume) map[string]interface{} {
	m := make(map[string]interface{})
	for _, v := range volumes {
		m[v.Name] = v
	}
	return m
}
