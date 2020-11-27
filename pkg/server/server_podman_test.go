package server

import (
	"github.com/containers/libpod/v2/libpod/define"
	"github.com/containers/libpod/v2/pkg/bindings/containers"
	"github.com/containers/libpod/v2/pkg/bindings/pods"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/runtime"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)
// Here are E2E test for server endpoints and podman runtime.
// Before running those you need to:
// 1. Ensure that podman.socket is open for superuser by running sudo systemctl start podman.socket
// 2. Ensure that sudo podman pod ps doesn't return itzpod - that's a constant that we use for pod created by itzo.
// 3. Executing of those test may take longer as they're E2E; using podman API we create, run, stop and remove pods and containers here.
//    All podman resources should be removed after each test with removeContainersAndPods function.

var (
	stateStopped = define.ContainerStateStopped
	stateRunning = define.ContainerStateRunning
	podmanAPITimeout uint = 5
	forceContainerRemove = true
	removeVolumes = true
)

func setUpServerAndController() (*Server, *PodController, error) {
	tmpdir, err := ioutil.TempDir("", "itzo-test-podman")
	if err != nil {
		panic("Error creating temporary directory")
	}
	defer os.RemoveAll(tmpdir)
	podctl, err := NewPodController(tmpdir, true)
	testServer := NewTestServer(EnvStore{}, tmpdir, podctl)
	podctl.Start()
	return &testServer, podctl, err
}

func tearDownSeverAndController() {
	KillChildren()
	return
}

func removeContainersAndPod(containersNames []string) {
	conn, err := runtime.GetPodmanConnection()
	if err != nil {
		panic(err)
	}
	for _, name := range containersNames {
		if !strings.HasPrefix(name, api.PodName) {
			name = api.PodName + "-" + name
		}
		err = containers.Stop(conn, name, &podmanAPITimeout)
		if err != nil {
			panic(err)
		}
		_, err = containers.Wait(conn, name, &stateStopped)
		if err != nil {
			panic(err)
		}
		err = containers.Remove(conn, name, &forceContainerRemove, &removeVolumes)
		if err != nil {
			panic(err)
		}
	}

	timeout := int(podmanAPITimeout)
	_, err = pods.Stop(conn, api.PodName, &timeout)
	if err != nil {
		panic(err)
	}
	_, err = pods.Remove(conn, api.PodName, &forceContainerRemove)
	if err != nil {
		panic(err)
	}
}

func TestCreatePodWithPodman(t *testing.T) {
	if !*testAgainstPodman {
		return
	}
	// GIVEN
	_, podCtl, err := setUpServerAndController()
	assert.NoError(t, err)
	defer tearDownSeverAndController()
	params := api.PodParameters{
		Secrets:     nil,
		Credentials: nil,
		Spec:        api.PodSpec{
			RestartPolicy:    api.RestartPolicyAlways,
			Units:            []api.Unit{
				{
					Name: "unit1",
					Image: "alpine:latest",
					Command: []string{"echo", "Hello Milpa"},
				},
			},
			InitUnits: []api.Unit{},
		},
	}
	assert.Equal(t, 0, len(podCtl.podStatus.Units))

	// WHEN
	podCtl.doUpdate(&params)

	// THEN
	assert.Equal(t, 1, len(podCtl.podStatus.Units))
	createdUnit := podCtl.podStatus.Units[0]
	assert.Equal(t, createdUnit.Name, "unit1")
	assert.Equal(t, createdUnit.Image, "alpine:latest")
	conn, err := runtime.GetPodmanConnection()
	assert.NoError(t, err)
	runningState := define.ContainerStateRunning
	_, err = containers.Wait(conn, "itzopod-unit1", &runningState)
	assert.NoError(t, err)

	removeContainersAndPod([]string{"unit1"})
}

func TestGetLogsWithPodman(t *testing.T) {
	if !*testAgainstPodman {
		return
	}
	// GIVEN
	testServer, podCtl, err := setUpServerAndController()
	assert.NoError(t, err)
	defer tearDownSeverAndController()
	params := api.PodParameters{
		Secrets:     nil,
		Credentials: nil,
		Spec:        api.PodSpec{
			RestartPolicy:    api.RestartPolicyNever,
			Units:            []api.Unit{
				{
					Name: "unit1",
					Image: "busybox:latest",
					Command: []string{
						"printf",
						"1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n",
						"&&",
						"sleep",
						"1000",
					},
				},
			},
			InitUnits: []api.Unit{},
		},
	}
	assert.Equal(t, 0, len(podCtl.podStatus.Units))


	podCtl.doUpdate(&params)
	assert.Equal(t, 1, len(podCtl.podStatus.Units))
	createdUnit := podCtl.podStatus.Units[0]
	assert.Equal(t, createdUnit.Name, "unit1")
	assert.Equal(t, createdUnit.Image, "busybox:latest")
	conn, err := runtime.GetPodmanConnection()
	assert.NoError(t, err)
	_, err = containers.Wait(conn, "itzopod-unit1", &stateStopped)
	assert.NoError(t, err)

	// WHEN
	req, err := http.NewRequest("GET", "/rest/v1/logs/unit1?lines=5&bytes=0", nil)
	assert.Nil(t, err)
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	testServer.ServeHTTP(rr, req)

	// THEN
	assert.Equal(t, http.StatusOK, rr.Code)
	responseBody := rr.Body.String()
	lines := strings.Split(responseBody, "\n")
	assert.Equal(t, []string{"6", "7", "8", "9", "10", ""}, lines)

	removeContainersAndPod([]string{"unit1"})
}