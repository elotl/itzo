package convert

import (
	"github.com/containers/libpod/v2/libpod/define"
	"github.com/elotl/itzo/pkg/api"
	"k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"path/filepath"
)

const ResolvconfVolumeName string = "resolvconf"

const EtchostsVolumeName string = "etchosts"

var (
	specialVolumesTypes = map[string]v1.HostPathType{
		ResolvconfVolumeName: v1.HostPathFile,
		EtchostsVolumeName:   v1.HostPathDirectory,
	}
	specialVolumesPaths = map[string]string{
		ResolvconfVolumeName: "/tmp/itzo/packages/resolvconf/etc/resolv.conf",
	}
)

func VolumeToK8sVolume(volume api.Volume) v1.Volume {
	var hostPathType v1.HostPathType
	if hostType, ok := specialVolumesTypes[volume.Name]; ok {
		hostPathType = hostType

	} else if volume.HostPath != nil {
		hostPathType = v1.HostPathType(*volume.HostPath.Type)
	}
	path := filepath.Join("/tmp/itzo/units", "..", "packages", volume.Name)
	if hostPath, ok := specialVolumesPaths[volume.Name]; ok {
		path = hostPath
	} else if volume.HostPath != nil {
		path = volume.HostPath.Path
	}

	vol := v1.Volume{
		Name: volume.Name,
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: path,
				Type: &hostPathType,
			},
		},
	}
	return vol
}

func UnitEnvToK8sContainerEnv(env api.EnvVar) v1.EnvVar {
	envVar := v1.EnvVar{
		Name:  env.Name,
		Value: env.Value,
	}
	if env.ValueFrom != nil {
		envVar.ValueFrom = &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: env.ValueFrom.SecretKeyRef.Name},
				Key:                  env.ValueFrom.SecretKeyRef.Key,
			},
		}
	}

	return envVar
}

func VolumeMountToK8sVolumeMount(vm api.VolumeMount) v1.VolumeMount {
	return v1.VolumeMount{
		Name:             vm.Name,
		ReadOnly:         false,
		MountPath:        vm.MountPath,
		SubPath:          vm.SubPath,
		MountPropagation: nil,
		SubPathExpr:      "",
	}
}

func SecurityCtxToK8sSecurityCtx(sc *api.SecurityContext) v1.SecurityContext {
	if sc == nil {
		return v1.SecurityContext{}
	}
	var add []v1.Capability
	var drop []v1.Capability
	for _, a := range sc.Capabilities.Add {
		add = append(add, v1.Capability(a))
	}
	for _, d := range sc.Capabilities.Drop {
		drop = append(drop, v1.Capability(d))
	}
	return v1.SecurityContext{
		Capabilities: &v1.Capabilities{
			Add:  add,
			Drop: drop,
		},
		RunAsUser:  sc.RunAsUser,
		RunAsGroup: sc.RunAsGroup,
	}
}

func ProbeToK8sProbe(probe *api.Probe) v1.Probe {
	if probe != nil {
		var headers []v1.HTTPHeader
		for _, header := range probe.HTTPGet.HTTPHeaders {
			headers = append(headers, v1.HTTPHeader{
				Name:  header.Name,
				Value: header.Value,
			})
		}
		return v1.Probe{
			Handler: v1.Handler{
				Exec: &v1.ExecAction{Command: probe.Exec.Command},
				HTTPGet: &v1.HTTPGetAction{
					Path: probe.HTTPGet.Path,
					Port: intstr.IntOrString{
						Type:   probe.Handler.HTTPGet.Port.Type,
						IntVal: probe.Handler.HTTPGet.Port.IntVal,
						StrVal: probe.Handler.HTTPGet.Port.StrVal,
					},
					Host:        probe.HTTPGet.Host,
					Scheme:      v1.URIScheme(probe.HTTPGet.Scheme),
					HTTPHeaders: headers,
				},
				TCPSocket: &v1.TCPSocketAction{
					Port: intstr.IntOrString{
						Type:   probe.Handler.TCPSocket.Port.Type,
						IntVal: probe.Handler.TCPSocket.Port.IntVal,
						StrVal: probe.Handler.TCPSocket.Port.StrVal,
					},
					Host: probe.Handler.TCPSocket.Host,
				},
			},
			InitialDelaySeconds: probe.InitialDelaySeconds,
			TimeoutSeconds:      probe.TimeoutSeconds,
			PeriodSeconds:       probe.PeriodSeconds,
			SuccessThreshold:    probe.SuccessThreshold,
			FailureThreshold:    probe.FailureThreshold,
		}
	}
	return v1.Probe{}
}

func UnitToK8sContainer(unit api.Unit) v1.Container {
	var ports []v1.ContainerPort
	for _, port := range unit.Ports {
		p := PortToK8sPortsports(port)
		ports = append(ports, p)
	}
	var envVars []v1.EnvVar
	for _, envVar := range unit.Env {
		envVars = append(envVars, UnitEnvToK8sContainerEnv(envVar))
	}
	var volMounts []v1.VolumeMount
	for _, volMount := range unit.VolumeMounts {
		volMounts = append(volMounts, VolumeMountToK8sVolumeMount(volMount))
	}
	readinessProbe := ProbeToK8sProbe(unit.ReadinessProbe)
	livenessProbe := ProbeToK8sProbe(unit.LivenessProbe)
	startupProbe := ProbeToK8sProbe(unit.StartupProbe)
	securityContext := SecurityCtxToK8sSecurityCtx(unit.SecurityContext)
	return v1.Container{
		Name:                     unit.Name,
		Image:                    unit.Image,
		Command:                  unit.Command,
		Args:                     unit.Args,
		WorkingDir:               unit.WorkingDir,
		Ports:                    ports,
		EnvFrom:                  nil,
		Env:                      envVars,
		Resources:                v1.ResourceRequirements{},
		VolumeMounts:             volMounts,
		VolumeDevices:            nil,
		LivenessProbe:            &livenessProbe,
		ReadinessProbe:           &readinessProbe,
		StartupProbe:             &startupProbe,
		Lifecycle:                nil,
		TerminationMessagePath:   "",
		TerminationMessagePolicy: "",
		ImagePullPolicy:          "",
		SecurityContext:          &securityContext,
		Stdin:                    false,
		StdinOnce:                false,
		TTY:                      false,
	}
}

func PortToK8sPortsports(port api.ContainerPort) v1.ContainerPort {
	return v1.ContainerPort{
		Name:          port.Name,
		HostIP:        port.HostIP,
		HostPort:      port.HostPort,
		ContainerPort: port.ContainerPort,
		Protocol:      v1.Protocol(port.Protocol),
	}
}

func PodDNSConfigtoK8sPodDNSConfig(dnsCfg api.PodDNSConfig) v1.PodDNSConfig {
	var options []v1.PodDNSConfigOption
	for _, opt := range dnsCfg.Options {
		options = append(options, v1.PodDNSConfigOption(opt))
	}
	return v1.PodDNSConfig{
		Nameservers: dnsCfg.Nameservers,
		Searches:    dnsCfg.Searches,
		Options:     options,
	}
}

func PodSecurityCtxToK8sPodSecurityCtx(podSecurityCtx api.PodSecurityContext) v1.PodSecurityContext {
	var systCtls []v1.Sysctl
	for _, sysCtl := range podSecurityCtx.Sysctls {
		systCtls = append(systCtls, v1.Sysctl(sysCtl))
	}
	return v1.PodSecurityContext{
		RunAsUser:          podSecurityCtx.RunAsUser,
		RunAsGroup:         podSecurityCtx.RunAsGroup,
		SupplementalGroups: podSecurityCtx.SupplementalGroups,
		Sysctls:            systCtls,
	}
}

func PodSpecToK8sPodSpec(podSpec api.PodSpec) v1.PodSpec {
	var volumes []v1.Volume
	for _, volume := range podSpec.Volumes {
		vol := VolumeToK8sVolume(volume)
		volumes = append(volumes, vol)
	}
	var initContainers []v1.Container
	for _, unit := range podSpec.InitUnits {
		container := UnitToK8sContainer(unit)
		initContainers = append(initContainers, container)
	}
	var containers []v1.Container
	for _, unit := range podSpec.Units {
		container := UnitToK8sContainer(unit)
		containers = append(containers, container)
	}
	var hostAliases []v1.HostAlias
	if podSpec.HostAliases != nil {
		for _, hostAlias := range podSpec.HostAliases {
			hA := v1.HostAlias(hostAlias)
			hostAliases = append(hostAliases, hA)
		}
	}
	var podDNSConfig v1.PodDNSConfig
	if podSpec.DNSConfig != nil {
		podDNSConfig = PodDNSConfigtoK8sPodDNSConfig(*podSpec.DNSConfig)
	}
	var podSecurityContext v1.PodSecurityContext
	if podSpec.SecurityContext != nil {
		podSecurityContext = PodSecurityCtxToK8sPodSecurityCtx(*podSpec.SecurityContext)
	}

	spec := v1.PodSpec{
		Volumes:          volumes,
		InitContainers:   initContainers,
		Containers:       containers,
		RestartPolicy:    v1.RestartPolicy(podSpec.RestartPolicy),
		DNSPolicy:        v1.DNSPolicy(podSpec.DNSPolicy),
		SecurityContext:  &podSecurityContext,
		ImagePullSecrets: nil,
		Hostname:         podSpec.Hostname,
		Subdomain:        podSpec.Subdomain,
		HostAliases:      hostAliases,
		DNSConfig:        &podDNSConfig,
	}
	return spec
}

type K8sPodYaml struct {
	Kind       string         `json:"kind"`
	ApiVersion string         `json:"apiVersion"`
	Spec       v1.PodSpec     `json:"spec"`
	Status     v1.PodStatus   `json:"status,omitempty"`
	TypeMeta   v12.TypeMeta   `json:",inline"`
	ObjectMeta v12.ObjectMeta `json:"metadata,omitempty"`
}

func K8sPodToYamlFormat(pod v1.PodSpec) K8sPodYaml {
	return K8sPodYaml{
		ApiVersion: "v1",
		Kind:       "Pod",
		Spec:       pod,
		Status:     v1.PodStatus{},
		TypeMeta:   v12.TypeMeta{},
		ObjectMeta: v12.ObjectMeta{
			// todo use constant here
			Name: api.PodName,
		},
	}
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
