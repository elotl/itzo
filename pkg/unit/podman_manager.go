package unit

import (
	"context"
	"github.com/containers/libpod/v2/libpod/define"
	"github.com/containers/libpod/v2/pkg/bindings"
	"github.com/containers/libpod/v2/pkg/bindings/containers"
	"github.com/containers/libpod/v2/pkg/bindings/images"
	"github.com/containers/libpod/v2/pkg/domain/entities"
	"github.com/containers/libpod/v2/pkg/specgen"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/golang/glog"
	"os"
)

type PodmanManager struct {}

func NewPodmanManager() *PodmanManager {

	return &PodmanManager{}
}

func (pm *PodmanManager) StartUnit(podname, hostname, unitname, workingdir, netns string, command, args, appenv []string, policy api.RestartPolicy) error {

	return nil
}

func (pm *PodmanManager) StopUnit(name string) error {
	return nil
}

func (pm *PodmanManager) RemoveUnit(name string) error  {
	return nil
}

func (pm *PodmanManager) GetLogBuffer(unit string) (*logbuf.LogBuffer, error) {
	return nil, nil
}

func (pm *PodmanManager) ReadLogBuffer(unit string, n int) ([]logbuf.LogEntry, error) {
	return nil, nil
}

func (pm *PodmanManager) UnitRunning(unit string) bool {
	return true
}

func (pm *PodmanManager) GetPid(unitName string) (int, bool)  {
	return 0, false
}

func (pm *PodmanManager) StartContainer(unitSpec api.Unit) error {
	// Get Podman socket location
	sock_dir := os.Getenv("XDG_RUNTIME_DIR")
	socket := "unix:" + sock_dir + "/podman/podman.sock"

	// Connect to Podman socket
	connText, err := bindings.NewConnection(context.Background(), socket)
	if err != nil {
		glog.Errorf("Error connecting to podman socket: %v", err)
		return err
	}
	_, err = images.Pull(connText, unitSpec.Image, entities.ImagePullOptions{})
	if err != nil {
		glog.Errorf("Error pulling unit image: %v", err)
		return err
	}

	// List images
	imageSummary, err := images.List(connText, nil, nil)
	if err != nil {
		return err
	}
	var names []string
	for _, i := range imageSummary {
		names = append(names, i.RepoTags...)
	}
	glog.Infof("podman images: %s", names)

	// Container create
	s := specgen.NewSpecGenerator(unitSpec.Image, false)
	s.Terminal = true
	r, err := containers.CreateWithSpec(connText, s)
	if err != nil {
		glog.Errorf("error creating with spec: %v", err)
		return err
	}

	// Container start
	err = containers.Start(connText, r.ID, nil)
	if err != nil {
		glog.Errorf("during starting container %s error occured: %v", r.ID, err)
		return err
	}

	// Wait for container to run
	running := define.ContainerStateRunning
	_, err = containers.Wait(connText, r.ID, &running)
	if err != nil {
		return err
	}

	// Container list
	var latestContainers = 1
	containerLatestList, err := containers.List(connText, nil, nil, &latestContainers, nil, nil, nil)
	if err != nil {
		glog.Error(err)
		return err
	}
	glog.Infof("Latest container is %s\n", containerLatestList[0].Names[0])

	// Container inspect
	ctrData, err := containers.Inspect(connText, r.ID, nil)
	if err != nil {
		glog.Errorf("error while inspecting %s : %v", r.ID, err)
		return err
	}
	glog.Infof("Container uses image %s\n", ctrData.ImageName)
	return nil
}
