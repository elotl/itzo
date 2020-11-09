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

package api

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func IsHostNetwork(securityContext *PodSecurityContext) bool {
	if securityContext == nil {
		return false
	}
	if securityContext.NamespaceOptions == nil ||
		securityContext.NamespaceOptions.Network != NamespaceModeNode {
		return false
	}
	return true
}

func MakeStillCreatingStatus(name, image, reason string) *UnitStatus {
	return &UnitStatus{
		Name: name,
		State: UnitState{
			Waiting: &UnitStateWaiting{
				Reason: reason,
			},
		},
		RestartCount: 0,
		Image:        image,
	}
}

func VolumeToK8sVolume(volume Volume) v1.Volume {
	sizeLimit := &resource.Quantity{}
	if volume.EmptyDir != nil {
		sizeLimit = resource.NewQuantity(volume.EmptyDir.SizeLimit, resource.DecimalSI)
	}
	var secretItems []v1.KeyToPath
	var configMapItems []v1.KeyToPath
	var projectionSources []v1.VolumeProjection
	if volume.Projected != nil {
		for _, source := range volume.Projected.Sources {
			var items []v1.KeyToPath
			for _, item := range source.Secret.Items {
				items = append(items, v1.KeyToPath(item))
			}
			var cmItems []v1.KeyToPath
			for _, item := range source.ConfigMap.Items {
				cmItems = append(cmItems, v1.KeyToPath(item))
			}
			projectionSources = append(projectionSources, v1.VolumeProjection{
				Secret:              &v1.SecretProjection{
					LocalObjectReference: v1.LocalObjectReference(source.Secret.LocalObjectReference),
					Items:                items,
					Optional:             source.Secret.Optional,
				},
				ConfigMap:           &v1.ConfigMapProjection{
					LocalObjectReference: v1.LocalObjectReference(source.ConfigMap.LocalObjectReference),
					Items:                cmItems,
					Optional:             source.ConfigMap.Optional,
				},
			})
		}
	}
	if volume.Secret != nil {
		for _, item := range volume.Secret.Items {
			secretItems = append(secretItems, v1.KeyToPath(item))
		}
	}
	if volume.ConfigMap != nil {
		for _, item := range volume.ConfigMap.Items {
			configMapItems = append(configMapItems, v1.KeyToPath(item))
		}
	}
	hostPath := v1.HostPathVolumeSource{}
	if volume.HostPath != nil {
		var hostPathType v1.HostPathType
		hostPathType = v1.HostPathType(*volume.HostPath.Type)
		hostPath = v1.HostPathVolumeSource{
			Path: volume.HostPath.Path,
			Type: &hostPathType,
		}
	}
	var emptyDir v1.EmptyDirVolumeSource
	if volume.EmptyDir != nil {
		emptyDir = v1.EmptyDirVolumeSource{
			Medium:    v1.StorageMedium(volume.EmptyDir.Medium),
			SizeLimit: sizeLimit,
		}
	}
	var configMap v1.ConfigMapVolumeSource
	if volume.ConfigMap != nil {
		configMap = v1.ConfigMapVolumeSource{
			LocalObjectReference: v1.LocalObjectReference{Name: volume.ConfigMap.LocalObjectReference.Name},
			Items:                configMapItems,
			DefaultMode:          volume.ConfigMap.DefaultMode,
			Optional:             volume.ConfigMap.Optional,
		}
	}
	var secretSource v1.SecretVolumeSource
	if volume.Secret != nil {
		secretSource = v1.SecretVolumeSource{
			SecretName:  volume.Secret.SecretName,
			Items:       secretItems,
			DefaultMode: volume.Secret.DefaultMode,
			Optional:    volume.Secret.Optional,
		}
	}
	var projectedSource v1.ProjectedVolumeSource
	if volume.Projected != nil {
		projectedSource = v1.ProjectedVolumeSource{
			Sources:     projectionSources,
			DefaultMode: volume.Projected.DefaultMode,
		}
	}

	vol := v1.Volume{
		Name: volume.Name,
		VolumeSource: v1.VolumeSource{
			HostPath:  &hostPath,
			EmptyDir:  &emptyDir,
			Secret:    &secretSource,
			ConfigMap: &configMap,
			Projected: &projectedSource,
		},
	}
	return vol
}

func UnitEnvToK8sContainerEnv(env EnvVar) v1.EnvVar {
	envVar := v1.EnvVar{
		Name:      env.Name,
		Value:     env.Value,
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

func VolumeMountToK8sVolumeMount(vm VolumeMount) v1.VolumeMount {
	return v1.VolumeMount{
		Name:             vm.Name,
		ReadOnly:         false,
		MountPath:        vm.MountPath,
		SubPath:          "",
		MountPropagation: nil,
		SubPathExpr:      "",
	}
}

func SecurityCtxToK8sSecurityCtx(sc *SecurityContext) v1.SecurityContext {
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

func ProbeToK8sProbe(probe *Probe) v1.Probe {
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

func UnitToK8sContainer(unit Unit) v1.Container {
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

func PortToK8sPortsports(port ContainerPort) v1.ContainerPort {
	return v1.ContainerPort{
		Name:          port.Name,
		HostIP:        port.HostIP,
		HostPort:      port.HostPort,
		ContainerPort: port.ContainerPort,
		Protocol:      v1.Protocol(port.Protocol),
	}
}

func PodDNSConfigtoK8sPodDNSConfig(dnsCfg PodDNSConfig) v1.PodDNSConfig {
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

func PodSecurityCtxToK8sPodSecurityCtx(podSecurityCtx PodSecurityContext) v1.PodSecurityContext {
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

func PodSpecToK8sPodSpec(podSpec PodSpec) v1.PodSpec {
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
	Kind string  `json:"kind"`
	ApiVersion string `json:"apiVersion"`
	Spec v1.PodSpec
	Status v1.PodStatus `yaml:"status,omitempty"`
	TypeMeta   metav1.TypeMeta
	ObjectMeta metav1.ObjectMeta
}

func K8sPodToYamlFormat(pod v1.PodSpec) K8sPodYaml {
	return K8sPodYaml{
		ApiVersion: "v1",
		Kind: "Pod",
		Spec: pod,
		Status:     v1.PodStatus{},
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			// todo use constant here
			Name: "my-pod",
		},
	}
}
