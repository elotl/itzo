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

package runtime

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/metrics"
)

type PodSandbox interface {
	RunPodSandbox(spec *api.PodSpec) error
	StopPodSandbox(spec *api.PodSpec) error
	RemovePodSandbox(spec *api.PodSpec) error
}

type ContainerService interface {
	CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) (*api.UnitStatus, error)
	StartContainer(unit api.Unit, spec *api.PodSpec, podName string) (*api.UnitStatus, error)
	RemoveContainer(unit *api.Unit) error
	ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error)
	//ExecSync()
	//Exec()
	//Attach()
	//PortForward()
	// TODO:: those below are needed for server handlers, we should think about ways to remove them from this interface
	GetLogBuffer(options LogOptions) (*logbuf.LogBuffer, error)
	UnitRunning(unitName string) bool
	GetPid(unitName string) (int, bool)
	SetPodNetwork(netNS, podIP string)
}

// This is heavily based on Kubernetes CRI.
// Methods we don't really need are commented out.
// Having it that similar to CRI opens a door for using it in the future.
type RuntimeService interface {
	PodSandbox
	ContainerService
	metrics.MetricsProvider
}

type ImageService interface {
	PullImage(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) error
}
