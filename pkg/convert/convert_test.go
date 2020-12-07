package convert

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"os"
	"strings"
	"testing"
)

func TestConvertPackagePathToHostPath(t *testing.T) {
	tmpFile, err := ioutil.TempFile(os.TempDir(), "itzo-test-packagepath-*")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	volName := strings.Replace(tmpFile.Name(), "/tmp/", "", 1)
	packagePath := api.PackagePath{Path: volName}
	hostPath, err := convertPackagePathToHostPath(packagePath, "/tmp")
	assert.NoError(t, err)
	expectedType := v1.HostPathFile
	assert.Equal(t, expectedType, *hostPath.Type)
	assert.Equal(t, tmpFile.Name(), hostPath.Path)
}
