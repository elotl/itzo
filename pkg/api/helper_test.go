package api

import (
	"testing"
)

func TestPodSpecToK8sPodSpec(t *testing.T)  {
	kipPodSpec := PodSpec{
		Phase:            PodRunning,
		RestartPolicy:    "Never",
		Units:            []Unit{
			{
				Name: "web",
				Image: "nginx:stable",
				Command: []string{},
				Args: []string{},
				Env: []EnvVar{
					{
						"KUBERNETES_SERVICE_PORT_HTTPS",
						"443",
						nil,
					},

				},
				VolumeMounts: []VolumeMount{
					{
						Name: "default-token-hppdm",
						MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
					},
				},
				Ports: []ContainerPort{
					{
						Name: "web",
						HostPort: 0,
						ContainerPort: 80,
						Protocol: MakeProtocol("TCP"),
					},
				},
			},
		},
		InitUnits:        []Unit{},
		ImagePullSecrets: nil,
		InstanceType:     "e2-micro",
		Spot:             PodSpot{Policy: SpotNever},
		Resources:        ResourceSpec{
			DedicatedCPU: false,
			SustainedCPU: nil,
			PrivateIPOnly: false,
		},
		Placement:        PlacementSpec{},
		Volumes:          []Volume{
			{
				Name: "default-token-hppdm",
				VolumeSource: VolumeSource{
					EmptyDir:    nil,
					PackagePath: nil,
					ConfigMap:   nil,
					Secret:      &SecretVolumeSource{
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
		SecurityContext:  &PodSecurityContext{
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