package runtime

import (
	"flag"
	"github.com/elotl/itzo/pkg/api"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

var tosiIntegration = flag.Bool("tosi-integration", false, "this runs itzo <-> tosi integration tests")

func TestMain(m *testing.M) {
	flag.Parse()
	ret := m.Run()
	os.Exit(ret)
}

func TestImagePuller_PullImagePublicImage(t *testing.T) {
	if !*tosiIntegration {
		t.Log("test skipped")
		return
	}
	ip := &ImagePuller{}
	os.MkdirAll("/tmp/itzo-pull-test", 77)
	registryCreds := make(map[string]api.RegistryCredentials, 0)
	err := ip.PullImage("/tmp", "unit-1", "library/hello-world", registryCreds, false)
	assert.NoError(t, err)
}

func TestImagePuller_PullImagePrivateImageFromECR(t *testing.T) {
	if !*tosiIntegration {
		t.Log("test skipped")
		return
	}
	ip := &ImagePuller{}
	ecrUser := os.Getenv("TOSI_TEST_DOCKER_USERNAME")
	ecrPass := os.Getenv("TOSI_TEST_DOCKER_PASS")
	if ecrUser == "" || ecrPass == "" {
		t.Fatalf("please set TOSI_TEST_DOCKER_USERNAME & TOSI_TEST_DOCKER_PASS env vars")
	}
	os.MkdirAll("/tmp/itzo-pull-test", 0700)
	registryCreds := make(map[string]api.RegistryCredentials, 0)
	registryCreds["689494258501.dkr.ecr.us-east-1.amazonaws.com"] = api.RegistryCredentials{
		Server:   "689494258501.dkr.ecr.us-east-1.amazonaws.com",
		Username: ecrUser,
		Password: ecrPass,
	}
	err := ip.PullImage("/tmp", "unit-2", "689494258501.dkr.ecr.us-east-1.amazonaws.com/helloserver:latest", registryCreds, false)
	assert.NoError(t, err)
}
