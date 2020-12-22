package server

import (
	"encoding/json"
	"fmt"
	"github.com/containers/libpod/v2/libpod/define"
	"github.com/containers/libpod/v2/pkg/bindings/containers"
	"github.com/containers/libpod/v2/pkg/bindings/pods"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/runtime/podman"
	"github.com/elotl/wsstream"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
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
	conn, err := podman.GetPodmanConnection()
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
	defer removeContainersAndPod([]string{"unit1"})
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
	conn, err := podman.GetPodmanConnection()
	assert.NoError(t, err)
	runningState := define.ContainerStateRunning
	_, err = containers.Wait(conn, "itzopod-unit1", &runningState)
	assert.NoError(t, err)
}

func TestGetLogsWithPodman(t *testing.T) {
	if !*testAgainstPodman {
		return
	}
	// GIVEN
	testServer, podCtl, err := setUpServerAndController()
	assert.NoError(t, err)
	defer tearDownSeverAndController()
	defer removeContainersAndPod([]string{"unit1"})
	params := api.PodParameters{
		Secrets:     nil,
		Credentials: nil,
		Spec:        api.PodSpec{
			RestartPolicy:    api.RestartPolicyNever,
			Units:            []api.Unit{
				{
					Name: "unit1",
					Image: "busybox:latest",
					Command: []string{"sh", "-c", "yes | head -n 10"},
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
	conn, err := podman.GetPodmanConnection()
	assert.NoError(t, err)
	_, err = containers.Wait(conn, "itzopod-unit1", &stateStopped)
	assert.NoError(t, err)

	// WHEN
	req, err := http.NewRequest("GET", "/rest/v1/logs/unit1?lines=5&bytes=0&metadata=1", nil)
	assert.Nil(t, err)
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	testServer.ServeHTTP(rr, req)

	// THEN
	assert.Equal(t, http.StatusOK, rr.Code)
	responseBody := rr.Body.String()
	lines := strings.Split(responseBody, "\n")
	for _, line := range lines[:len(lines)-1] {
		l := strings.Split(line, " ")
		assert.Equal(t,"stdout", l[1])
		assert.Equal(t, "F", l[2])
		assert.Equal(t, "y", l[3])
		_, err = time.Parse(time.RFC3339Nano, l[0])
		assert.NoError(t, err)
	}
}

func TestGetStatusWithPodman(t *testing.T)  {
	if !*testAgainstPodman {
		return
	}

	// GIVEN
	testServer, podCtl, err := setUpServerAndController()
	assert.NoError(t, err)
	defer tearDownSeverAndController()
	defer removeContainersAndPod([]string{"unit1", "unit2"})
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
						"sleep",
						"1000",
					},
				},
				{
					Name: "unit2",
					Image: "busybox:latest",
					Command: []string{
						"echo",
						"I'm done",
					},
				},
			},
			InitUnits: []api.Unit{},
		},
	}
	assert.Equal(t, 0, len(podCtl.podStatus.Units))


	podCtl.doUpdate(&params)
	assert.Equal(t, 2, len(podCtl.podStatus.Units))
	createdUnit := podCtl.podStatus.Units[0]
	assert.Equal(t, "unit1", createdUnit.Name)
	assert.Equal(t,  "busybox:latest", createdUnit.Image)
	conn, err := podman.GetPodmanConnection()
	assert.NoError(t, err)
	_, err = containers.Wait(conn, "itzopod-unit1", &stateRunning)
	assert.NoError(t, err)
	_, err = containers.Wait(conn, "itzopod-unit2", &stateStopped)
	assert.NoError(t, err)

	// WHEN
	req, err := http.NewRequest("GET", "/rest/v1/status", nil)
	assert.Nil(t, err)
	rr := httptest.NewRecorder()
	testServer.ServeHTTP(rr, req)

	// THEN
	assert.Equal(t, http.StatusOK, rr.Code)
	var podStatus api.PodStatusReply
	body := rr.Body.Bytes()
	err = json.Unmarshal(body, &podStatus)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(podStatus.UnitStatuses))
	assert.Equal(t, 0, len(podStatus.InitUnitStatuses))
	assert.Equal(t, "unit1", podStatus.UnitStatuses[0].Name)
	assert.NotNil(t, podStatus.UnitStatuses[0].State.Running)
	assert.Equal(t, "stopped", podStatus.UnitStatuses[1].State.Waiting.Reason)
	assert.Equal(t, false, podStatus.UnitStatuses[1].State.Waiting.StartFailure)
}

func TestPortForwardWithPodman(t *testing.T) {
	if !*testAgainstPodman {
		return
	}

	// GIVEN
	testServer, podCtl, err := setUpServerAndController()
	assert.NoError(t, err)
	defer tearDownSeverAndController()
	defer removeContainersAndPod([]string{"unit1"})
	params := api.PodParameters{
		Secrets:     nil,
		Credentials: nil,
		Spec:        api.PodSpec{
			RestartPolicy:    api.RestartPolicyNever,
			Units:            []api.Unit{
				{
					Name: "unit1",
					Image: "jmalloc/echo-server",
					Ports: []api.ContainerPort{
						{
							HostPort:      10000,
							ContainerPort: 10000,
							Protocol:      "TCP",
						},
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
	assert.Equal(t, "unit1", createdUnit.Name)
	assert.Equal(t,  "jmalloc/echo-server", createdUnit.Image)
	conn, err := podman.GetPodmanConnection()
	assert.NoError(t, err)
	_, err = containers.Wait(conn, "itzopod-unit1", &stateRunning)
	assert.NoError(t, err)

	// WHEN

	testServer.getHandlers()
	testServer.httpServer = &http.Server{Addr: ":0", Handler: testServer}
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	go testServer.httpServer.Serve(listener)
	portstr := fmt.Sprintf("%d", port)
	ws, err := createWebsocketClient(portstr, "/rest/v1/portforward/")
	assert.NoError(t, err)
	pfp := api.PortForwardParams{
		Port: portstr,
	}
	pfpb, err := json.Marshal(pfp)
	assert.NoError(t, err)
	err = ws.WriteRaw(pfpb)
	assert.NoError(t, err)

	msg := []byte("GET /rest/v1/ping HTTP/1.1\nHost: localhost:" + portstr + "\r\n\r\n")
	err = ws.WriteMsg(0, msg)
	assert.NoError(t, err)
	timeout := 3 * time.Second
	select {
	case f := <-ws.ReadMsg():
		_, m, err := wsstream.UnpackMessage(f)
		assert.NoError(t, err)
		assert.Len(t, m, 0)
	case <-time.After(timeout):
		assert.FailNow(t, "reading timed out")
	}
	select {
	case f := <-ws.ReadMsg():
		c, m, err := wsstream.UnpackMessage(f)
		assert.NoError(t, err)
		assert.Equal(t, 1, c)
		assert.True(t, strings.HasSuffix(string(m), "pong"))
	case <-time.After(timeout):
		assert.FailNow(t, "reading timed out")
	}
}
