package convert

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/ghodss/yaml"
	"github.com/golangplus/testing/assert"
	"github.com/instrumenta/kubeval/kubeval"
	"testing"
)

func TestPodSpecToK8sPodSpec(t *testing.T)  {
	kipPodSpec := api.PodSpec{
		Phase:         api.PodRunning,
		RestartPolicy: "Never",
		Units:            []api.Unit{
			{
				Name: "web",
				Image: "nginx:stable",
				Command: []string{},
				Args: []string{},
				Env: []api.EnvVar{
					{
						"KUBERNETES_SERVICE_PORT_HTTPS",
						"443",
						nil,
					},

				},
				VolumeMounts: []api.VolumeMount{
					{
						Name: "default-token-hppdm",
						MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
						SubPath: "",
					},
				},
				Ports: []api.ContainerPort{
					{
						Name:          "web",
						HostPort:      0,
						ContainerPort: 80,
						Protocol:      api.MakeProtocol("TCP"),
					},
				},
			},
		},
		InitUnits:        []api.Unit{},
		ImagePullSecrets: nil,
		InstanceType:     "e2-micro",
		Spot:             api.PodSpot{Policy: api.SpotNever},
		Resources:        api.ResourceSpec{
			DedicatedCPU: false,
			SustainedCPU: nil,
			PrivateIPOnly: false,
		},
		Placement: api.PlacementSpec{},
		Volumes:          []api.Volume{
			{
				Name: "default-token-hppdm",
				VolumeSource: api.VolumeSource{
					EmptyDir:    nil,
					PackagePath: nil,
					ConfigMap:   nil,
					Secret:      &api.SecretVolumeSource{
						SecretName:  "secret",
						Items:       nil,
						DefaultMode: nil,
						Optional:    nil,
					},
					HostPath:    nil,
					Projected:   nil,
				},
			},
		},
		SecurityContext:  &api.PodSecurityContext{
			NamespaceOptions:   nil,
			RunAsUser:          nil,
			RunAsGroup:         nil,
			SupplementalGroups: nil,
			Sysctls:            nil,
		},
		DNSPolicy:        "",
		DNSConfig:        nil,
		Hostname:         "",
		Subdomain:        "",
		HostAliases:      nil,
	}
	PodSpecToK8sPodSpec(kipPodSpec)
}

func TestK8sPodToYamlFormat(t *testing.T) {
	kipPodSpec := api.PodSpec{
		Phase:         api.PodRunning,
		RestartPolicy: "Never",
		Units:            []api.Unit{
			{
				Name: "web",
				Image: "nginx:stable",
				Command: []string{},
				Args: []string{},
				Env: []api.EnvVar{
					{
						"KUBERNETES_SERVICE_PORT_HTTPS",
						"443",
						nil,
					},

				},
				VolumeMounts: []api.VolumeMount{
					{
						Name: "default-token-hppdm",
						MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
						SubPath: "",
					},
					{
						Name: ResolvconfVolumeName,
						MountPath: "/etc/resolvconf",
						SubPath: "/etc/resolv.conf",
					},
				},
				Ports: []api.ContainerPort{
					{
						Name:          "web",
						HostPort:      0,
						ContainerPort: 80,
						Protocol:      api.MakeProtocol("TCP"),
					},
				},
			},
		},
		InitUnits:        []api.Unit{},
		ImagePullSecrets: nil,
		InstanceType:     "e2-micro",
		Spot:             api.PodSpot{Policy: api.SpotNever},
		Resources:        api.ResourceSpec{
			DedicatedCPU: false,
			SustainedCPU: nil,
			PrivateIPOnly: false,
		},
		Placement: api.PlacementSpec{},
		Volumes:          []api.Volume{
			{
				Name: "default-token-hppdm",
				VolumeSource: api.VolumeSource{
					EmptyDir:    nil,
					PackagePath: nil,
					ConfigMap:   nil,
					Secret:      &api.SecretVolumeSource{
						SecretName:  "secret",
						Items:       nil,
						DefaultMode: nil,
						Optional:    nil,
					},
					HostPath:    nil,
					Projected:   nil,
				},
			},
			{
				Name: ResolvconfVolumeName,
				VolumeSource: api.VolumeSource{
					HostPath: &api.HostPathVolumeSource{
						Path: "/tmp/itzo/packages/resolvconf",
						Type: nil,
					} ,
				},
			},
		},
		SecurityContext:  &api.PodSecurityContext{
			NamespaceOptions:   nil,
			RunAsUser:          nil,
			RunAsGroup:         nil,
			SupplementalGroups: nil,
			Sysctls:            nil,
		},
		DNSPolicy:        "",
		DNSConfig:        nil,
		Hostname:         "",
		Subdomain:        "",
		HostAliases:      nil,
	}
	k8sPodSpec := PodSpecToK8sPodSpec(kipPodSpec)
	podYaml := K8sPodToYamlFormat(k8sPodSpec)
	fileContents, err := yaml.Marshal(podYaml)
	t.Log(string(fileContents))
	assert.NoError(t, err)
	conf := kubeval.NewDefaultConfig()
	_, err = kubeval.Validate(fileContents, conf)
	assert.NoError(t, err)
}