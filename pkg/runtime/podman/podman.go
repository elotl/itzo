// +build !darwin

/*
Copyright 2020 Elotl Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package podman

import (
	"context"
	"errors"
	"github.com/containers/podman/v2/pkg/bindings"
	"github.com/containers/podman/v2/pkg/bindings/containers"
	"github.com/containers/podman/v2/pkg/bindings/images"
	"github.com/containers/podman/v2/pkg/bindings/pods"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/containers/podman/v2/pkg/specgen"
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/convert"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/metrics"
	"github.com/elotl/itzo/pkg/runtime"
	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	v1 "k8s.io/api/core/v1"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultPodmanSocketPath string = "unix:/run/podman/podman.sock"
	defaultTimeout          uint   = 30
	RestartPolicyNo                = "no"
	// RestartPolicyAlways unconditionally restarts the container.
	RestartPolicyAlways = "always"
	// RestartPolicyOnFailure restarts the container on non-0 exit code,
	// with an optional maximum number of retries.
	RestartPolicyOnFailure = "on-failure"
)

var (
	restartPolicyMap = map[api.RestartPolicy]string{
		api.RestartPolicyAlways:    RestartPolicyAlways,
		api.RestartPolicyOnFailure: RestartPolicyOnFailure,
		api.RestartPolicyNever:     RestartPolicyNo,
	}
)

type PodmanSandbox struct {
	metrics.PodmanMetricsProvider

	connText context.Context
}

func (ps *PodmanSandbox) RunPodSandbox(spec *api.PodSpec) error {
	podSpec := specgen.NewPodSpecGenerator()
	podSpec.Name = api.PodName
	hostname := spec.Hostname
	if hostname == "" {
		hostname = api.PodName
	}
	nsMode := specgen.Default
	if api.IsHostNetwork(spec.SecurityContext) {
		nsMode = specgen.Host
	}
	podSpec.NetNS = specgen.Namespace{
		NSMode: nsMode,
		Value:  "",
	}
	podSpec.Hostname = hostname
	// those two are important as we deploy and mount resolv.conf and /etc/hosts
	podSpec.NoManageResolvConf = true
	podSpec.NoManageHosts = true
	portMappings := make([]specgen.PortMapping, 0)
	for _, unit := range spec.Units {
		unitPortMappings, err := convert.UnitPortsToPodmanPortMapping(unit.Ports)
		if err != nil {
			return err
		}
		// TODO we should ensure there's no port collision on host
		portMappings = append(portMappings, unitPortMappings...)
	}
	podSpec.PortMappings = portMappings
	_, err := pods.CreatePodFromSpec(ps.connText, podSpec)
	if err != nil {
		glog.Errorf("error creating pod from spec: %v", err)
	}
	return err
}

func (ps *PodmanSandbox) StopPodSandbox(spec *api.PodSpec) error {
	report, err := pods.Stop(ps.connText, api.PodName, nil)
	if report != nil && len(report.Errs) > 0 {
		return errors.New("TODO")
	}
	return err
}
func (ps *PodmanSandbox) RemovePodSandbox(spec *api.PodSpec) error {
	report, err := pods.Remove(ps.connText, api.PodName, nil)
	if report != nil && report.Err != nil {
		return report.Err
	}
	return err
}

type PodmanImageService struct {
	connText context.Context
}

func (p *PodmanImageService) PullImage(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) error {
	var server, _, err = util.ParseImageSpec(image)
	if err != nil {
		glog.Errorf("error parsing image spec %s: %v", image, err)
		return err
	}
	exists, err := images.Exists(p.connText, image)
	if err != nil {
		glog.Errorf("error checking if image %s already exists: %v", image, err)
		return err
	}
	if exists {
		glog.Infof("image %s already exists", image)
		return nil
	}

	username, password := util.GetRepoCreds(server, registryCredentials)
	opts := entities.ImagePullOptions{
		Username: username,
		Password: password,
	}
	glog.Infof("trying to pull image: %s for container: %s", image, name)
	_, err = images.Pull(p.connText, image, opts)
	return err
}

func (p *PodmanImageService) RemoveImage(rootdir, image string) error {
	_, err := images.Remove(p.connText, image, false)
	return err
}

type PodmanContainerService struct {
	rootdir   string
	imgPuller PodmanImageService
}

func NewPodmanContainerService(ctx context.Context, rootdir string) *PodmanContainerService {
	return &PodmanContainerService{rootdir: rootdir, imgPuller: PodmanImageService{connText: ctx}}
}

func (pcs *PodmanContainerService) CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) (*api.UnitStatus, error) {
	container := convert.UnitToContainer(unit, nil)

	var err = pcs.imgPuller.PullImage(pcs.rootdir, unit.Name, unit.Image, registryCredentials, false)
	if err != nil {
		glog.Errorf("pulling image %s for container %s failed with: %v", unit.Image, unit.Name, err)
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "Pulling image failed"), err
	}
	containerSpec := specgen.NewSpecGenerator(container.Image, false)
	containerSpec.Name = convert.UnitNameToContainerName(unit.Name)
	containerSpec.Pod = api.PodName
	containerSpec.Command = unit.Command
	containerSpec.RestartPolicy = restartPolicyMap[spec.RestartPolicy]
	containerSpec.Env = make(map[string]string)
	containerSpec.Mounts = make([]runtimespec.Mount, 0)

	// TODO - examine if it's needed
	for _, env := range container.Env {
		if env.ValueFrom == nil {
			containerSpec.Env[env.Name] = env.Value
		}
	}
	for _, mount := range container.VolumeMounts {
		var volume v1.Volume
		for _, vol := range spec.Volumes {
			if vol.Name == mount.Name {
				volumePtr := convert.MilpaToK8sVolume(vol)
				volume = *volumePtr
				break
			}
		}
		path := filepath.Join("/tmp/itzo/packages", volume.Name)
		if volume.HostPath != nil {
			path = volume.HostPath.Path
		}

		if mount.SubPath != "" {
			path = filepath.Join(path, mount.SubPath)
		}
		containerSpec.Mounts = append(containerSpec.Mounts, runtimespec.Mount{
			Destination: mount.MountPath,
			Type:        "bind",
			Source:      path,
			Options:     nil,
		})
	}
	_, err = containers.CreateWithSpec(pcs.imgPuller.connText, containerSpec)
	if err != nil {
		glog.Errorf("error from contrainer.CreateWithSpec: %v", err)
		//return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "podman failed to start container"), err
	}
	glog.Infof("podman started unit %s as container %s with %s image", unit.Name, container.Name, unit.Image)
	return api.MakeStillCreatingStatus(unit.Name, unit.Image, "Container created"), nil
}

func (pcs *PodmanContainerService) StartContainer(unit api.Unit, spec *api.PodSpec, podName string) (*api.UnitStatus, error) {
	podmanContainerName := convert.UnitNameToContainerName(unit.Name)
	err := containers.Start(pcs.imgPuller.connText, podmanContainerName, nil)
	if err != nil {
		return api.MakeFailedUpdateStatus(unit.Name, unit.Image, "runtime cannot start container"), err
	}
	return nil, nil
}

func (pcs *PodmanContainerService) RemoveContainer(unit *api.Unit) error {
	containerName := convert.UnitNameToContainerName(unit.Name)
	err := containers.Stop(pcs.imgPuller.connText, containerName, nil)
	if err != nil {
		return err
	}
	force := true
	deleteVolumes := false
	err = containers.Remove(pcs.imgPuller.connText, containerName, &force, &deleteVolumes)
	return err
}

func (pcs *PodmanContainerService) ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error) {
	containerName := convert.UnitNameToContainerName(unitName)
	ctrData, err := containers.Inspect(pcs.imgPuller.connText, containerName, nil)
	if err != nil {
		// TODO - distinguish situations:
		// A - container died (for whatever reason)
		// B - container is stooped by pod controller and will be recreated with newly image (because it got updated by user)
		// if we return failed status kip marks pod as failed and reschedules it on new instance
		//errorMsg := fmt.Sprintf("cannot get container state from podman for unit: %s\n%v \nctrData: %v", containerName, err, ctrData)
		return api.MakeStillCreatingStatus(unitName, ctrData.Image, "Container stopped"), nil
		//return api.MakeFailedUpdateStatus(unitName, ctrData.Image, "Container runtime failure"), errors.New(errorMsg)

	}
	state, ready := convert.ContainerStateToUnit(*ctrData)
	status := &api.UnitStatus{
		Name:         unitName,
		State:        state,
		RestartCount: 0,
		Image:        unitImage,
		Ready:        ready,
	}
	return status, nil
}

type PodmanRuntime struct {
	PodmanSandbox
	PodmanContainerService
}

func (p *PodmanRuntime) GetLogBuffer(options runtime.LogOptions) (*logbuf.LogBuffer, error) {
	tail := 4096
	if options.LineNum != 0 {
		tail = options.LineNum
	}
	logBuf := logbuf.NewLogBuffer(tail)
	containerName := convert.UnitNameToContainerName(options.UnitName)
	yes := true
	tailStr := strconv.Itoa(tail)
	out := make(chan string)
	opts := containers.LogOptions{
		Follow:     &options.Follow,
		Stderr:     &yes,
		Stdout:     &yes,
		Tail:       &tailStr,
		Timestamps: &yes,
	}
	go func() {
		err := containers.Logs(p.connText, containerName, opts, out, out)
		if err != nil {
			glog.Errorf("cannot get logs for container %s : %v", containerName, err)
		}
		close(out)
	}()
	for msg := range out {
		logLine := strings.Split(msg, " ")
		line := ""
		if len(logLine) > 1 {
			line = strings.Join(logLine[1:], "")
		} else {
			continue
		}
		logBuf.Write(logbuf.StdoutLogSource, line+"\n", &logLine[0])
	}
	return logBuf, nil
}

func (p *PodmanRuntime) UnitRunning(unitName string) bool {
	unitStatus, err := p.ContainerStatus(unitName, "")
	if err != nil {
		glog.Error(err)
		return false
	}
	if unitStatus.State.Running == nil {
		return false
	}
	return true
}

func (p *PodmanRuntime) GetPid(unitName string) (int, bool) {
	return 0, false
}

func (p *PodmanRuntime) SetPodNetwork(netNS, podIP string) {
	return
}

func NewRuntime(rootdir string) (*PodmanRuntime, error) {
	connText, err := GetPodmanConnection()
	if err != nil {
		return nil, err
	}
	containerService := NewPodmanContainerService(connText, rootdir)
	return &PodmanRuntime{
		PodmanSandbox:          PodmanSandbox{connText: connText},
		PodmanContainerService: *containerService,
	}, nil
}

func GetPodmanConnection() (context.Context, error) {
	var podmanSocketPath = os.Getenv("PODMAN_SOCKET_PATH")
	if podmanSocketPath == "" {
		podmanSocketPath = defaultPodmanSocketPath
	}
	connText, err := bindings.NewConnection(context.Background(), podmanSocketPath)
	return connText, err
}
