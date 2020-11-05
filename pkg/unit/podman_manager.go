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
)

const (
	PodmanSocketPath = "unix:/run/podman/podman.sock"
)

type PodmanManager struct {
	ConnText context.Context
	rootdir  string
}

func GetPodmanConnection() (context.Context, error) {
	// Connect to Podman socket
	connText, err := bindings.NewConnection(context.Background(), PodmanSocketPath)
	return connText, err
}

func NewPodmanManager(rootdir string) (*PodmanManager, error) {
	// Connect to Podman socket
	connText, err := GetPodmanConnection()
	if err != nil {
		glog.Errorf("Error connecting to podman socket: %v", err)
		return &PodmanManager{}, err
	}
	return &PodmanManager{ConnText: connText, rootdir: rootdir}, nil
}

func (pm *PodmanManager) GetContext() context.Context {
	return pm.ConnText
}

func (pm *PodmanManager) StartUnit(podname, hostname, unitname, workingdir, netns string, command, args, appenv []string, policy api.RestartPolicy) error {
	unitSpec, err := OpenUnit(pm.rootdir, unitname)
	if err != nil {
		glog.Errorf("error opening unit: %v", err)
		return err
	}
	glog.Infof("pulling podman image: %s", unitSpec.Image)
	_, err = images.Pull(pm.ConnText, unitSpec.Image, entities.ImagePullOptions{})
	if err != nil {
		glog.Errorf("Error pulling unit image: %v", err)
		return err
	}
	glog.Infof("successfully pulled image: %s", unitSpec.Image)

	// List images
	imageSummary, err := images.List(pm.ConnText, nil, nil)
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
	r, err := containers.CreateWithSpec(pm.ConnText, s)
	if err != nil {
		glog.Errorf("error creating with spec: %v", err)
		return err
	}

	// Container start
	err = containers.Start(pm.ConnText, r.ID, nil)
	if err != nil {
		glog.Errorf("during starting container %s error occured: %v", r.ID, err)
		return err
	}

	// Wait for container to run
	running := define.ContainerStateRunning
	_, err = containers.Wait(pm.ConnText, r.ID, &running)
	if err != nil {
		return err
	}

	// Container list
	var latestContainers = 1
	containerLatestList, err := containers.List(pm.ConnText, nil, nil, &latestContainers, nil, nil, nil)
	if err != nil {
		glog.Error(err)
		return err
	}
	glog.Infof("Latest container is %s\n", containerLatestList[0].Names[0])

	// Container inspect
	ctrData, err := containers.Inspect(pm.ConnText, r.ID, nil)
	if err != nil {
		glog.Errorf("error while inspecting %s : %v", r.ID, err)
		return err
	}
	glog.Infof("Container uses image %s\n", ctrData.ImageName)
	return nil
}

func (pm *PodmanManager) StopUnit(name string) error {
	err := containers.Stop(pm.ConnText, name, nil)
	return err
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

	// Container inspect
	ctrData, err := containers.Inspect(pm.ConnText, unit, nil)
	if err != nil {
		glog.Errorf("error while inspecting %s : %v", unit, err)
		return false
	}
	return ctrData.State.Running
}

func (pm *PodmanManager) GetPid(unitName string) (int, bool)  {
	return 0, false
}

func (pm *PodmanManager) StartContainer(rootdir, name string) error {
	unitSpec, err := OpenUnit(rootdir, name)
	if err != nil {
		glog.Errorf("error opening unit: %v", err)
		return err
	}
	glog.Infof("pulling podman image: %s", unitSpec.Image)
	_, err = images.Pull(pm.ConnText, unitSpec.Image, entities.ImagePullOptions{})
	if err != nil {
		glog.Errorf("Error pulling unit image: %v", err)
		return err
	}
	glog.Infof("successfully pulled image: %s", unitSpec.Image)

	// List images
	imageSummary, err := images.List(pm.ConnText, nil, nil)
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
	r, err := containers.CreateWithSpec(pm.ConnText, s)
	if err != nil {
		glog.Errorf("error creating with spec: %v", err)
		return err
	}

	// Container start
	err = containers.Start(pm.ConnText, r.ID, nil)
	if err != nil {
		glog.Errorf("during starting container %s error occured: %v", r.ID, err)
		return err
	}

	// Wait for container to run
	running := define.ContainerStateRunning
	_, err = containers.Wait(pm.ConnText, r.ID, &running)
	if err != nil {
		return err
	}

	// Container list
	var latestContainers = 1
	containerLatestList, err := containers.List(pm.ConnText, nil, nil, &latestContainers, nil, nil, nil)
	if err != nil {
		glog.Error(err)
		return err
	}
	glog.Infof("Latest container is %s\n", containerLatestList[0].Names[0])

	// Container inspect
	ctrData, err := containers.Inspect(pm.ConnText, r.ID, nil)
	if err != nil {
		glog.Errorf("error while inspecting %s : %v", r.ID, err)
		return err
	}
	glog.Infof("Container uses image %s\n", ctrData.ImageName)
	return nil
}