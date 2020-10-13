package podman

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/containers/buildah/pkg/parse"
	"github.com/containers/podman/v2/libpod"
	"github.com/containers/podman/v2/libpod/define"
	"github.com/containers/podman/v2/libpod/image"
	ann "github.com/containers/podman/v2/pkg/annotations"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/containers/podman/v2/pkg/domain/infra"
	envLib "github.com/containers/podman/v2/pkg/env"
	ns "github.com/containers/podman/v2/pkg/namespaces"
	createconfig "github.com/containers/podman/v2/pkg/spec"
	"github.com/containers/podman/v2/pkg/specgen/generate"
	"github.com/containers/podman/v2/pkg/util"
	"github.com/containers/storage"
	"github.com/cri-o/ocicni/pkg/ocicni"
	"github.com/docker/distribution/reference"
	"github.com/elotl/itzo/pkg/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

const (
	// https://kubernetes.io/docs/concepts/storage/volumes/#hostpath
	kubeDirectoryPermission = 0755
	// https://kubernetes.io/docs/concepts/storage/volumes/#hostpath
	kubeFilePermission = 0644
)

type Podman struct {
	libpodRuntime *libpod.Runtime
}

// kubeSeccompPaths holds information about a pod YAML's seccomp configuration
// it holds both container and pod seccomp paths
type kubeSeccompPaths struct {
	containerPaths map[string]string
	podPath        string
}

func NewPodman() (*Podman, error) {
	flags := &pflag.FlagSet{}        // TODO
	opts := &entities.PodmanConfig{} // TODO
	r, err := infra.GetRuntime(context.Background(), flags, opts)
	if err != nil {
		return nil, err
	}
	return &Podman{
		libpodRuntime: r,
	}, nil
}

func (pm *Podman) CreatePod(ctx context.Context, podName string, podSpec *api.PodSpec) error {
	var (
		pod *libpod.Pod
		//registryCreds *types.DockerAuthConfig
		writer io.Writer
	)

	// check for name collision between pod and container
	if podName == "" {
		return errors.Errorf("pod does not have a name")
	}
	for _, n := range podSpec.Units {
		if n.Name == podName {
			return errors.Errorf("a container exists with the same name (%q) as the pod", podName)
		}
	}

	podOptions := []libpod.PodCreateOption{
		libpod.WithInfraContainer(),
		libpod.WithPodName(podName),
	}

	// TODO we only configure Process namespace. We also need to account for Host{IPC,Network,PID}
	// which is not currently possible with pod create
	//if podSpec.ShareProcessNamespace != nil && *podSpec.ShareProcessNamespace {
	//	podOptions = append(podOptions, libpod.WithPodPID())
	//}

	hostname := podSpec.Hostname
	if hostname == "" {
		hostname = podName
	}
	podOptions = append(podOptions, libpod.WithPodHostname(hostname))

	//if podSpec.HostNetwork {
	//	podOptions = append(podOptions, libpod.WithPodHostNetwork())
	//}

	if podSpec.HostAliases != nil {
		hosts := make([]string, 0, len(podSpec.HostAliases))
		for _, hostAlias := range podSpec.HostAliases {
			for _, host := range hostAlias.Hostnames {
				hosts = append(hosts, host+":"+hostAlias.IP)
			}
		}
		podOptions = append(podOptions, libpod.WithPodHosts(hosts))
	}

	nsOptions, err := generate.GetNamespaceOptions(strings.Split(createconfig.DefaultKernelNamespaces, ","))
	if err != nil {
		return err
	}
	podOptions = append(podOptions, nsOptions...)
	podPorts := getPodPorts(podSpec.Units)
	podOptions = append(podOptions, libpod.WithInfraContainerPorts(podPorts))

	// Create the Pod
	pod, err = pm.libpodRuntime.NewPod(ctx, podOptions...)
	if err != nil {
		return err
	}

	podInfraID, err := pod.InfraContainerID()
	if err != nil {
		return err
	}
	hasUserns := false
	if podInfraID != "" {
		podCtr, err := pm.libpodRuntime.GetContainer(podInfraID)
		if err != nil {
			return err
		}
		mappings, err := podCtr.IDMappings()
		if err != nil {
			return err
		}
		hasUserns = len(mappings.UIDMap) > 0
	}

	namespaces := map[string]string{
		// Disabled during code review per mheon
		//"pid":  fmt.Sprintf("container:%s", podInfraID),
		"net": fmt.Sprintf("container:%s", podInfraID),
		"ipc": fmt.Sprintf("container:%s", podInfraID),
		"uts": fmt.Sprintf("container:%s", podInfraID),
	}
	if hasUserns {
		namespaces["user"] = fmt.Sprintf("container:%s", podInfraID)
	}

	//if len(options.Username) > 0 && len(options.Password) > 0 {
	//	registryCreds = &types.DockerAuthConfig{
	//		Username: options.Username,
	//		Password: options.Password,
	//	}
	//}
	dockerRegistryOptions := image.DockerRegistryOptions{
		//	DockerRegistryCreds:         registryCreds,
		//	DockerCertPath:              options.CertDir,
		//	DockerInsecureSkipTLSVerify: options.SkipTLSVerify,
	}

	// map from name to mount point
	volumes := make(map[string]string)
	for _, volume := range podSpec.Volumes {
		hostPath := volume.VolumeSource.HostPath
		if hostPath == nil {
			return errors.Errorf("HostPath is currently the only supported VolumeSource")
		}
		if hostPath.Type != nil {
			switch *hostPath.Type {
			case api.HostPathDirectoryOrCreate:
				if _, err := os.Stat(hostPath.Path); os.IsNotExist(err) {
					if err := os.Mkdir(hostPath.Path, kubeDirectoryPermission); err != nil {
						return errors.Errorf("error creating HostPath %s", volume.Name)
					}
				}
				// Label a newly created volume
				if err := libpod.LabelVolumePath(hostPath.Path); err != nil {
					return errors.Wrapf(err, "error giving %s a label", hostPath.Path)
				}
			case api.HostPathFileOrCreate:
				if _, err := os.Stat(hostPath.Path); os.IsNotExist(err) {
					f, err := os.OpenFile(hostPath.Path, os.O_RDONLY|os.O_CREATE, kubeFilePermission)
					if err != nil {
						return errors.Errorf("error creating HostPath %s", volume.Name)
					}
					if err := f.Close(); err != nil {
						logrus.Warnf("Error in closing newly created HostPath file: %v", err)
					}
				}
				// unconditionally label a newly created volume
				if err := libpod.LabelVolumePath(hostPath.Path); err != nil {
					return errors.Wrapf(err, "error giving %s a label", hostPath.Path)
				}
			case api.HostPathSocket:
				st, err := os.Stat(hostPath.Path)
				if err != nil {
					return errors.Wrap(err, "error checking HostPathSocket")
				}
				if st.Mode()&os.ModeSocket != os.ModeSocket {
					return errors.Errorf("error checking HostPathSocket: path %s is not a socket", hostPath.Path)
				}

			case api.HostPathDirectory:
			case api.HostPathFile:
			case api.HostPathUnset:
				// do nothing here because we will verify the path exists in validateVolumeHostDir
				break
			default:
				return errors.Errorf("Invalid HostPath type %v", hostPath.Type)
			}
		}

		if err := parse.ValidateVolumeHostDir(hostPath.Path); err != nil {
			return errors.Wrapf(err, "error in parsing HostPath in YAML")
		}
		volumes[volume.Name] = hostPath.Path
	}

	var ctrRestartPolicy string
	switch podSpec.RestartPolicy {
	case api.RestartPolicyAlways:
		ctrRestartPolicy = libpod.RestartPolicyAlways
	case api.RestartPolicyOnFailure:
		ctrRestartPolicy = libpod.RestartPolicyOnFailure
	case api.RestartPolicyNever:
		ctrRestartPolicy = libpod.RestartPolicyNo
	default: // Default to Always
		ctrRestartPolicy = libpod.RestartPolicyAlways
	}

	containers := make([]*libpod.Container, 0, len(podSpec.Units))
	for _, container := range podSpec.Units {
		pullPolicy := util.PullImageMissing
		//if len(container.ImagePullPolicy) > 0 {
		//	pullPolicy, err = util.ValidatePullType(string(container.ImagePullPolicy))
		//	if err != nil {
		//		return err
		//	}
		//}
		named, err := reference.ParseNormalizedNamed(container.Image)
		if err != nil {
			return err
		}
		// In kube, if the image is tagged with latest, it should always pull
		if tagged, isTagged := named.(reference.NamedTagged); isTagged {
			if tagged.Tag() == image.LatestTag {
				pullPolicy = util.PullImageAlways
			}
		}
		signaturePolicyFilePath := "" // TODO
		authFilePath := ""            // TODO
		newImage, err := pm.libpodRuntime.ImageRuntime().New(ctx, container.Image, signaturePolicyFilePath, authFilePath, writer, &dockerRegistryOptions, image.SigningOptions{}, nil, pullPolicy)
		if err != nil {
			return err
		}
		conf, err := kubeContainerToCreateConfig(ctx, container, newImage, namespaces, volumes, pod.ID(), podName, podInfraID, nil)
		if err != nil {
			return err
		}
		conf.RestartPolicy = ctrRestartPolicy
		ctr, err := createconfig.CreateContainerFromCreateConfig(ctx, pm.libpodRuntime, conf, pod)
		if err != nil {
			return err
		}
		containers = append(containers, ctr)
	}

	// start the containers
	for _, ctr := range containers {
		if err := ctr.Start(ctx, true); err != nil {
			// Making this a hard failure here to avoid a mess
			// the other containers are in created status
			return err
		}
	}

	//return pod.ID()

	return nil
}

func (pm *Podman) GetContainerStatus(podName, containerName string) (*api.UnitStatus, error) {
	pod, err := pm.libpodRuntime.LookupPod(podName)
	if err != nil {
		return nil, err
	}
	ctrs, err := pod.AllContainers()
	if err != nil {
		return nil, err
	}
	for _, ctr := range ctrs {
		if ctr.IsInfra() {
			continue
		}
		if ctr.Name() != containerName {
			continue
		}
		ctrState, err := ctr.ContainerState()
		if err != nil {
			return nil, err
		}
		_, imageName := ctr.Image()
		started := false
		stateStr := fmt.Sprintf("%v", ctrState.State)
		state := api.UnitState{}
		switch ctrState.State {
		case define.ContainerStateUnknown, define.ContainerStateConfigured, define.ContainerStateCreated:
			state.Waiting = &api.UnitStateWaiting{
				Reason:       stateStr,
				StartFailure: false, // XXX: when is this true?
			}
		case define.ContainerStateRunning, define.ContainerStatePaused:
			started = true
			state.Running = &api.UnitStateRunning{
				StartedAt: api.Time{
					Time: ctrState.StartedTime,
				},
			}
		case define.ContainerStateStopped, define.ContainerStateExited, define.ContainerStateRemoving:
			state.Terminated = &api.UnitStateTerminated{
				ExitCode: ctrState.ExitCode,
				FinishedAt: api.Time{
					Time: ctrState.FinishedTime,
				},
				Reason:  stateStr,
				Message: stateStr,
				StartedAt: api.Time{
					Time: ctrState.StartedTime,
				},
			}
		}
		status := api.UnitStatus{
			Name:         containerName,
			RestartCount: int32(ctrState.RestartCount),
			Image:        imageName,
			Started:      &started,
			Ready:        started, // XXX: implement probes.
			State:        state,
			//TODO LastTerminationState
		}
		return &status, nil
	}
	return nil, errors.Errorf("container %q not found", containerName)
}

// getPodPorts converts a slice of kube container descriptions to an
// array of ocicni portmapping descriptions usable in libpod.
func getPodPorts(containers []api.Unit) []ocicni.PortMapping {
	var infraPorts []ocicni.PortMapping
	for _, container := range containers {
		for _, p := range container.Ports {
			if p.HostPort != 0 && p.ContainerPort == 0 {
				p.ContainerPort = p.HostPort
			}
			if p.Protocol == "" {
				p.Protocol = "tcp"
			}
			portBinding := ocicni.PortMapping{
				HostPort:      p.HostPort,
				ContainerPort: p.ContainerPort,
				Protocol:      strings.ToLower(string(p.Protocol)),
				HostIP:        p.HostIP,
			}
			// Only hostPort is utilized in podman context, all container ports
			// are accessible inside the shared network namespace.
			if p.HostPort != 0 {
				infraPorts = append(infraPorts, portBinding)
			}
		}
	}
	return infraPorts
}

// kubeContainerToCreateConfig takes a Container and returns a createconfig
// describing a container.
func kubeContainerToCreateConfig(ctx context.Context, containerYAML api.Unit, newImage *image.Image, namespaces map[string]string, volumes map[string]string, podID, podName, infraID string, seccompPaths *kubeSeccompPaths) (*createconfig.CreateConfig, error) {
	var (
		containerConfig createconfig.CreateConfig
		pidConfig       createconfig.PidConfig
		networkConfig   createconfig.NetworkConfig
		cgroupConfig    createconfig.CgroupConfig
		utsConfig       createconfig.UtsConfig
		ipcConfig       createconfig.IpcConfig
		userConfig      createconfig.UserConfig
		securityConfig  createconfig.SecurityConfig
	)

	// The default for MemorySwappiness is -1, not 0
	containerConfig.Resources.MemorySwappiness = -1

	containerConfig.Image = containerYAML.Image
	containerConfig.ImageID = newImage.ID()

	// podName should be non-empty for Deployment objects to be able to create
	// multiple pods having containers with unique names
	if podName == "" {
		return nil, errors.Errorf("kubeContainerToCreateConfig got empty podName")
	}
	containerConfig.Name = fmt.Sprintf("%s-%s", podName, containerYAML.Name)

	//containerConfig.Tty = containerYAML.TTY

	containerConfig.Pod = podID

	imageData, _ := newImage.Inspect(ctx)

	userConfig.User = "0"
	if imageData != nil {
		userConfig.User = imageData.Config.User
	}

	//setupSecurityContext(&securityConfig, &userConfig, containerYAML)

	// Since we prefix the container name with pod name to work-around the uniqueness requirement,
	// the seccom profile should reference the actual container name from the YAML
	// but apply to the containers with the prefixed name
	//securityConfig.SeccompProfilePath = seccompPaths.findForContainer(containerYAML.Name)

	containerConfig.Command = []string{}
	if imageData != nil && imageData.Config != nil {
		containerConfig.Command = imageData.Config.Entrypoint
	}
	if len(containerYAML.Command) != 0 {
		containerConfig.Command = containerYAML.Command
	}
	// doc https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#notes
	if len(containerYAML.Args) != 0 {
		containerConfig.Command = append(containerConfig.Command, containerYAML.Args...)
	} else if len(containerYAML.Command) == 0 {
		// Add the Cmd from the image config only if containerYAML.Command and containerYAML.Args are empty
		containerConfig.Command = append(containerConfig.Command, imageData.Config.Cmd...)
	}
	if imageData != nil && len(containerConfig.Command) == 0 {
		return nil, errors.Errorf("No command specified in container YAML or as CMD or ENTRYPOINT in this image for %s", containerConfig.Name)
	}

	containerConfig.UserCommand = containerConfig.Command

	containerConfig.StopSignal = 15

	containerConfig.WorkDir = "/"
	if imageData != nil {
		// FIXME,
		// we are currently ignoring imageData.Config.ExposedPorts
		containerConfig.BuiltinImgVolumes = imageData.Config.Volumes
		if imageData.Config.WorkingDir != "" {
			containerConfig.WorkDir = imageData.Config.WorkingDir
		}
		containerConfig.Labels = imageData.Config.Labels
		if imageData.Config.StopSignal != "" {
			stopSignal, err := util.ParseSignal(imageData.Config.StopSignal)
			if err != nil {
				return nil, err
			}
			containerConfig.StopSignal = stopSignal
		}
	}

	if containerYAML.WorkingDir != "" {
		containerConfig.WorkDir = containerYAML.WorkingDir
	}
	// If the user does not pass in ID mappings, just set to basics
	if userConfig.IDMappings == nil {
		userConfig.IDMappings = &storage.IDMappingOptions{}
	}

	networkConfig.NetMode = ns.NetworkMode(namespaces["net"])
	ipcConfig.IpcMode = ns.IpcMode(namespaces["ipc"])
	utsConfig.UtsMode = ns.UTSMode(namespaces["uts"])
	// disabled in code review per mheon
	//containerConfig.PidMode = ns.PidMode(namespaces["pid"])
	userConfig.UsernsMode = ns.UsernsMode(namespaces["user"])
	if len(containerConfig.WorkDir) == 0 {
		containerConfig.WorkDir = "/"
	}

	containerConfig.Pid = pidConfig
	containerConfig.Network = networkConfig
	containerConfig.Uts = utsConfig
	containerConfig.Ipc = ipcConfig
	containerConfig.Cgroup = cgroupConfig
	containerConfig.User = userConfig
	containerConfig.Security = securityConfig

	annotations := make(map[string]string)
	if infraID != "" {
		annotations[ann.SandboxID] = infraID
		annotations[ann.ContainerType] = ann.ContainerTypeContainer
	}
	containerConfig.Annotations = annotations

	// Environment Variables
	envs := map[string]string{}
	if imageData != nil {
		imageEnv, err := envLib.ParseSlice(imageData.Config.Env)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing image environment variables")
		}
		envs = imageEnv
	}
	for _, env := range containerYAML.Env {
		envs[env.Name] = env.Value
	}
	containerConfig.Env = envs

	for _, volume := range containerYAML.VolumeMounts {
		var readonly string
		hostPath, exists := volumes[volume.Name]
		if !exists {
			return nil, errors.Errorf("Volume mount %s specified for container but not configured in volumes", volume.Name)
		}
		if err := parse.ValidateVolumeCtrDir(volume.MountPath); err != nil {
			return nil, errors.Wrapf(err, "error in parsing MountPath")
		}
		//if volume.ReadOnly {
		//	readonly = ":ro"
		//}
		containerConfig.Volumes = append(containerConfig.Volumes, fmt.Sprintf("%s:%s%s", hostPath, volume.MountPath, readonly))
	}
	return &containerConfig, nil
}
