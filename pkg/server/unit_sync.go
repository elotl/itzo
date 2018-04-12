package server

import (
	"fmt"
	"strings"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
)

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
				Reason: msg,
				// todo: add failure info
			},
		},
		Image: unit.Image,
	}
}

func SyncPodUnits(desired *api.PodSpec, current *api.PodSpec) map[string]api.UnitStatus {
	// By this point, spec must have had the secrets merged into the env vars
	erroredUnits := make(map[string]api.UnitStatus)

	spec := specToUnitMap(desired)
	status := specToUnitMap(current)
	add, update, delete := util.MapDiff(spec, status)

	// Updates need to be deleted and then added
	delete = append(delete, update...)
	add = append(add, update...)

	// do deletes
	for _, unitName := range delete {
		unit := status[unitName].(api.Unit)
		err := DeleteUnit(unit.Name)
		if err != nil {
			glog.Errorf("Error deleting unit %s, trying to continue", unit.Name)
		}
	}

	// Sync the volumes before adding new units
	SyncVolumes(desired, current)

	// if a delete fails, attempt to carry on.  If an add fails,
	// collect the units that failed and pass them back to the caller
	// so that we can update the unit's status and bubble up the data
	// to milpa.

	for _, unitName := range add {
		unit := spec[unitName].(api.Unit)

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
			err := attachMount(unit.Name, mount.Name, mount.MountPath)
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

		err = startUnitHelper(
			rootDir, unit, unit.Command, makeAppEnv(unit),
			RestartPolicy(pod.Spec.RestartPolicy))
		if err != nil {
			msg := fmt.Sprintf("Error starting unit %s: %v",
				unit.Name, err)
			erroredUnits[unit.Name] = makeFailedStatus(unit, msg)
			continue
		}
	}
	return erroredUnits
}

func pullImage(name, image, server, username, password string) error {
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
	rootfs := u.GetRootfs()

	err = pullAndExtractImage(image, rootfs, server, username, password)
	if err != nil {
		return fmt.Errorf("pulling image %s: %v", image, err)
	}
}

func attachMount(unitName, mountName, mountPath string) error {

}

func volumesToMap(volumes []api.Volume) map[string]interface{} {
	m := make(map[string]interface{})
	for _, v := range volumes {
		m[v.Name] = v
	}
	return m
}

func SyncVolumes(desired *api.PodSpec, current *api.PodSpec) {
	spec := volumesToMap(desired.Volumes)
	status := volumesToMap(current.Volumes)
	add, update, delete := util.MapDiff(spec, status)

	// Updates need to be deleted and then added
	delete = append(delete, update...)
	add = append(add, update...)

	// do deletes
	for _, volName := range delete {
		vol := status[volName].(api.Volume)
		err := DeleteMount(vol)
		if err != nil {
			glog.Errorf("Error removing volume %s: %v", vol.Name, err)
		}
	}

	// do adds
	for _, volName := range add {
		vol := spec[volName].(api.Volume)
		err := CreateMount(vol)
		if err != nil {
			glog.Errorf("Error creating volume: %s, %v", vol.Name, err)
		}
	}
}
