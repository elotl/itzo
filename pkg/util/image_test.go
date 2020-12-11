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
				"index.docker.io": api.RegistryCredentials{
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
				"registry-1.docker.io": api.RegistryCredentials{
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
