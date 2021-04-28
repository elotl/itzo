package util

import (
	"fmt"
	"github.com/elotl/itzo/pkg/api"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetRepoCreds(t *testing.T) {
	tests := []struct {
		server string
		creds  map[string]api.RegistryCredentials
		u      string
		p      string
	}{
		{
			server: "",
			creds:  nil,
			u:      "",
			p:      "",
		},
		{
			server: "",
			creds: map[string]api.RegistryCredentials{
				"index.docker.io": {
					Username: "myuser",
					Password: "mypass",
				},
			},
			u: "myuser",
			p: "mypass",
		},
		{
			server: "docker.io",
			creds: map[string]api.RegistryCredentials{
				"registry-1.docker.io": {
					Username: "myuser",
					Password: "mypass",
				},
			},
			u: "myuser",
			p: "mypass",
		},
	}
	for i, tc := range tests {
		user, pass := GetRepoCreds(tc.server, tc.creds)
		msg := fmt.Sprintf("test case %d failed", i)
		assert.Equal(t, tc.u, user, msg)
		assert.Equal(t, tc.p, pass, msg)
	}
}

func TestParseImageSpec(t *testing.T) {
	cases := []struct {
		name     string
		imageStr string
		server   string
		repo     string
	}{
		{
			name:     "ecr repo",
			imageStr: "689494258501.dkr.ecr.us-east-1.amazonaws.com/buildscaler:latest",
			server:   "689494258501.dkr.ecr.us-east-1.amazonaws.com",
			repo:     "buildscaler:latest",
		},
		{
			name:     "dockerhub with library",
			imageStr: "library/nginx:stable",
			server:   "",
			repo:     "library/nginx:stable",
		},
		{
			name:     "dockerhub without library",
			imageStr: "nginx:stable",
			server:   "",
			repo:     "library/nginx:stable",
		},
		{
			name:     "ecr repo without tag",
			imageStr: "689494258501.dkr.ecr.us-east-1.amazonaws.com/buildscaler",
			server:   "689494258501.dkr.ecr.us-east-1.amazonaws.com",
			repo:     "buildscaler",
		},
		{
			name:     "dockerhub with library without atg",
			imageStr: "library/nginx",
			server:   "",
			repo:     "library/nginx",
		},
		{
			name:     "dockerhub without library without tag",
			imageStr: "nginx",
			server:   "",
			repo:     "library/nginx",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, repo := ParseImageSpec(tc.imageStr)
			assert.Equal(t, tc.server, server)
			assert.Equal(t, tc.repo, repo)

		})
	}
}
