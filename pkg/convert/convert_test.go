package convert

import (
	"github.com/containers/libpod/v2/pkg/specgen"
	"github.com/elotl/itzo/pkg/api"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"os"
	"strings"
	"testing"
)

func TestConvertPackagePathToHostPath(t *testing.T) {
	dirName := "testitzovolume"
	tmpDir, err := ioutil.TempDir(os.TempDir(), dirName)
	volName := strings.TrimPrefix(tmpDir, "/tmp/")
	assert.NoError(t, err)
	tmpFile, err := ioutil.TempFile(tmpDir,  "itzo-test-packagepath-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	path := strings.TrimPrefix(tmpFile.Name(), tmpDir)
	packagePath := api.PackagePath{Path: path}
	hostPath, err := convertPackagePathToHostPath(packagePath, "/tmp", volName)
	assert.NoError(t, err)
	expectedType := v1.HostPathFile
	assert.Equal(t, expectedType, *hostPath.Type)
	assert.Equal(t, tmpFile.Name(), hostPath.Path)
}


func TestUnitPortsToPodmanPortMapping(t *testing.T) {
	testCases := []struct{
		name string
		unitPorts []api.ContainerPort
		portMappings []specgen.PortMapping
		shouldErr bool
	}{
		{
			name: "only container port set",
			unitPorts: []api.ContainerPort{
				{
					ContainerPort: 5000,
				},
			},
			portMappings: []specgen.PortMapping{
				{
					ContainerPort: 5000,
					HostPort: 5000,
				},
			},
		},
		{
			name: "container and host ports set",
			unitPorts: []api.ContainerPort{
				{
					ContainerPort: 5000,
					HostPort: 8000,
				},
			},
			portMappings: []specgen.PortMapping{
				{
					ContainerPort: 5000,
					HostPort: 8000,
				},
			},
		},
		{
			name: "with proto",
			unitPorts: []api.ContainerPort{
				{
					ContainerPort: 5000,
					HostPort: 8000,
					Protocol: "TCP",
				},
			},
			portMappings: []specgen.PortMapping{
				{
					ContainerPort: 5000,
					HostPort: 8000,
					Protocol: "tcp",
				},
			},
		},
		{
			name: "with hostPort:0",
			unitPorts: []api.ContainerPort{
				{
					ContainerPort: 5000,
					HostPort: 0,
				},
			},
			portMappings: []specgen.PortMapping{
				{
					ContainerPort: 5000,
					HostPort: 5000,
				},
			},
		},
		{
			name: "container port not set err",
			unitPorts: []api.ContainerPort{
				{
					HostPort: 8000,
					Protocol: "TCP",
				},
			},
			portMappings: nil,
			shouldErr: true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			portMappings, err := UnitPortsToPodmanPortMapping(testCase.unitPorts)
			if testCase.shouldErr {
				assert.Error(t, err)
			}
			assert.Equal(t, testCase.portMappings, portMappings)
		})
	}
}