package convert

import (
	"github.com/containers/libpod/v2/libpod/define"
	"github.com/elotl/itzo/pkg/api"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
	"path/filepath"
)

const (
	packagePathBaseDirectory = "/tmp/itzo/packages"
)

func MilpaToK8sVolume(vol api.Volume) *v1.Volume {
	convertKeyToPath := func(milpa []api.KeyToPath) []v1.KeyToPath {
		k8s := make([]v1.KeyToPath, len(milpa))
		for i := range milpa {
			k8s[i] = v1.KeyToPath{
				Key:  milpa[i].Key,
				Path: milpa[i].Path,
				Mode: milpa[i].Mode,
			}
		}
		return k8s
	}
	if vol.Secret != nil {
		return &v1.Volume{
			Name: vol.Name,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  vol.Secret.SecretName,
					Items:       convertKeyToPath(vol.Secret.Items),
					DefaultMode: vol.Secret.DefaultMode,
					Optional:    vol.Secret.Optional,
				},
			},
		}
	} else if vol.HostPath != nil {
		var hostPathTypePtr *v1.HostPathType
		if vol.HostPath.Type != nil {
			hostPathType := v1.HostPathType(string(*vol.HostPath.Type))
			hostPathTypePtr = &hostPathType
		}
		return &v1.Volume{
			Name: vol.Name,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: vol.HostPath.Path,
					Type: hostPathTypePtr,
				},
			},
		}
	} else if vol.ConfigMap != nil {
		return &v1.Volume{
			Name: vol.Name,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: vol.ConfigMap.Name,
					},
					Items:       convertKeyToPath(vol.ConfigMap.Items),
					DefaultMode: vol.ConfigMap.DefaultMode,
					Optional:    vol.ConfigMap.Optional,
				},
			},
		}
	} else if vol.EmptyDir != nil {
		var sizeLimit *resource.Quantity
		if vol.EmptyDir.SizeLimit != 0 {
			sizeLimit = resource.NewQuantity(vol.EmptyDir.SizeLimit, resource.BinarySI)
		}
		return &v1.Volume{
			Name: vol.Name,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{
					Medium:    v1.StorageMedium(string(vol.EmptyDir.Medium)),
					SizeLimit: sizeLimit,
				},
			},
		}
	} else if vol.Projected != nil {
		projVol := &v1.ProjectedVolumeSource{
			DefaultMode: vol.Projected.DefaultMode,
		}
		projVol.Sources = make([]v1.VolumeProjection, len(vol.Projected.Sources))
		for i, src := range vol.Projected.Sources {
			if src.Secret != nil {
				k8Secret := &v1.SecretProjection{
					LocalObjectReference: v1.LocalObjectReference{
						Name: src.Secret.Name,
					},
					Items:    convertKeyToPath(src.Secret.Items),
					Optional: src.Secret.Optional,
				}
				projVol.Sources[i].Secret = k8Secret
			} else if src.ConfigMap != nil {
				k8CM := &v1.ConfigMapProjection{
					LocalObjectReference: v1.LocalObjectReference{
						Name: src.ConfigMap.Name,
					},
					Items:    convertKeyToPath(src.ConfigMap.Items),
					Optional: src.ConfigMap.Optional,
				}
				projVol.Sources[i].ConfigMap = k8CM
			}
		}
		return &v1.Volume{
			Name: vol.Name,
			VolumeSource: v1.VolumeSource{
				Projected: projVol,
			},
		}
	} else if vol.PackagePath != nil {
		hostPathSource, err := convertPackagePathToHostPath(*vol.PackagePath, packagePathBaseDirectory, vol.Name)
		if err != nil {
			glog.Errorf("failed to convert vol.PackagePath %v to hostPath : %v", *vol.PackagePath, err)
		}
		return &v1.Volume{
			Name: vol.Name,
			VolumeSource: v1.VolumeSource{
				HostPath: &hostPathSource,
			},
		}
	} else {
		glog.Warningf("Unsupported volume type for volume: %s", vol.Name)
	}
	return nil
}

func milpaProbeToK8sProbe(mp *api.Probe) *v1.Probe {
	if mp == nil {
		return nil
	}
	kp := &v1.Probe{
		InitialDelaySeconds: mp.InitialDelaySeconds,
		TimeoutSeconds:      mp.TimeoutSeconds,
		PeriodSeconds:       mp.PeriodSeconds,
		SuccessThreshold:    mp.SuccessThreshold,
		FailureThreshold:    mp.FailureThreshold,
	}
	if mp.Exec != nil {
		kp.Exec = &v1.ExecAction{
			Command: mp.Exec.Command,
		}
	} else if mp.HTTPGet != nil {
		kp.HTTPGet = &v1.HTTPGetAction{
			Path:   mp.HTTPGet.Path,
			Port:   mp.HTTPGet.Port,
			Host:   mp.HTTPGet.Host,
			Scheme: v1.URIScheme(string(mp.HTTPGet.Scheme)),
		}
		h := make([]v1.HTTPHeader, len(mp.HTTPGet.HTTPHeaders))
		for i := range mp.HTTPGet.HTTPHeaders {
			h[i].Name = mp.HTTPGet.HTTPHeaders[i].Name
			h[i].Value = mp.HTTPGet.HTTPHeaders[i].Value
		}
		kp.HTTPGet.HTTPHeaders = h
	} else if mp.TCPSocket != nil {
		kp.TCPSocket = &v1.TCPSocketAction{
			Port: mp.TCPSocket.Port,
			Host: mp.TCPSocket.Host,
		}
	}
	return kp
}

func UnitToContainer(unit api.Unit, container *v1.Container) v1.Container {
	if container == nil {
		container = &v1.Container{}
	}
	container.Name = unit.Name
	container.Image = unit.Image
	container.Command = unit.Command
	container.Args = unit.Args
	container.WorkingDir = unit.WorkingDir
	container.Env = make([]v1.EnvVar, len(unit.Env))
	for i, e := range unit.Env {
		container.Env[i] = v1.EnvVar{
			Name:  e.Name,
			Value: e.Value,
		}
	}
	for _, port := range unit.Ports {
		container.Ports = append(container.Ports,
			v1.ContainerPort{
				Name:          port.Name,
				ContainerPort: port.ContainerPort,
				HostPort:      port.HostPort,
				Protocol:      v1.Protocol(string(port.Protocol)),
				HostIP:        port.HostIP,
			})
	}
	usc := unit.SecurityContext
	if usc != nil {
		if container.SecurityContext == nil {
			container.SecurityContext = &v1.SecurityContext{}
		}
		csc := container.SecurityContext
		csc.RunAsUser = usc.RunAsUser
		csc.RunAsGroup = usc.RunAsGroup
		ucaps := usc.Capabilities
		if ucaps != nil {
			caps := &v1.Capabilities{
				Add:  make([]v1.Capability, len(ucaps.Add)),
				Drop: make([]v1.Capability, len(ucaps.Drop)),
			}
			for i, a := range ucaps.Add {
				caps.Add[i] = v1.Capability(string(a))
			}
			for i, d := range ucaps.Drop {
				caps.Drop[i] = v1.Capability(string(d))
			}
			csc.Capabilities = caps
		}
	}
	for _, vm := range unit.VolumeMounts {
		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
			Name:      vm.Name,
			MountPath: vm.MountPath,
		})
	}
	container.StartupProbe = milpaProbeToK8sProbe(unit.StartupProbe)
	container.ReadinessProbe = milpaProbeToK8sProbe(unit.ReadinessProbe)
	container.LivenessProbe = milpaProbeToK8sProbe(unit.LivenessProbe)

	return *container
}

func UnitNameToContainerName(unitName string) string {
	return api.PodName + "-" + unitName
}

func ContainerStateToUnit(ctrData define.InspectContainerData) (api.UnitState, bool) {
	if ctrData.State != nil {
		state := *ctrData.State
		if state.Running {
			return api.UnitState{
				Running: &api.UnitStateRunning{StartedAt: api.Time{Time: state.StartedAt}},
			}, true
		}
		if state.Dead {
			return api.UnitState{
				Terminated: &api.UnitStateTerminated{
					ExitCode:   state.ExitCode,
					FinishedAt: api.Time{Time: state.FinishedAt},
					// TODO send better reason
					Reason:    state.Status,
					Message:   state.Error,
					StartedAt: api.Time{Time: state.StartedAt},
				},
			}, false
		}
		return api.UnitState{
			Waiting: &api.UnitStateWaiting{
				Reason:       state.Status,
				StartFailure: false,
			},
		}, false
	}
	return api.UnitState{Waiting: &api.UnitStateWaiting{
		Reason:       "waiting for container status",
		StartFailure: false,
	}}, false
}

func convertPackagePathToHostPath(hostPath api.PackagePath, itzoPackagesPath, volumeName string) (v1.HostPathVolumeSource, error) {
	// Volumes are deployed as files (or dirs) into
	// /tmp/itzo/packages/<volume-name>/<packagePath.Path>
	// to build correct host path, we need:
	// 1. itzoPackagePath, which is const (/tmp/itzo/packages) but to ease testing we pass it as first arg
	// 2. packagePath.Path
	// 3. volumeName
	path := filepath.Join(itzoPackagesPath, volumeName, hostPath.Path)
	info, err := os.Stat(path)
	if err != nil {
		return v1.HostPathVolumeSource{}, err
	}
	HostPathType := v1.HostPathFile
	if info.IsDir() {
		HostPathType = v1.HostPathDirectory
	}

	return v1.HostPathVolumeSource{
		Path: path,
		Type: &HostPathType,
	}, nil
}
