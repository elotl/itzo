package api

import (
	"testing"
)

func TestPodSpecToK8sPodSpec(t *testing.T)  {
	kipPodSpec := PodSpec{
		Phase:            "",
		RestartPolicy:    "Always",
		Units:            []Unit{
			{
				Name:                     "unit1",
				Image:                    "image1",
			},
		},
		InitUnits:        []Unit{},
		ImagePullSecrets: nil,
		InstanceType:     "",
		Spot:             PodSpot{},
		Resources:        ResourceSpec{},
		Placement:        PlacementSpec{},
		Volumes:          nil,
		SecurityContext:  nil,
		DNSPolicy:        "",
		DNSConfig:        nil,
		Hostname:         "",
		Subdomain:        "",
		HostAliases:      nil,
	}
	PodSpecToK8sPodSpec(kipPodSpec)
}