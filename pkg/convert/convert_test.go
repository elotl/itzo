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
