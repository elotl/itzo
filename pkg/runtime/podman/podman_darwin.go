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
	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/runtime"
)

type NoOpPodmanRuntime struct {}

func (n NoOpPodmanRuntime) GetLogBuffer(options runtime.LogOptions) (*logbuf.LogBuffer, error) {
	panic("implement me")
}

func (n NoOpPodmanRuntime) ReadSystemMetrics(s string) api.ResourceMetrics {
	panic("implement me")
}

func (n NoOpPodmanRuntime) ReadUnitMetrics(s string) api.ResourceMetrics {
	panic("implement me")
}

func (n NoOpPodmanRuntime) RunPodSandbox(spec *api.PodSpec) error {
	return nil
}

func (n NoOpPodmanRuntime) StopPodSandbox(spec *api.PodSpec) error {
	return nil
}

func (n NoOpPodmanRuntime) RemovePodSandbox(spec *api.PodSpec) error {
	return nil
}

func (n NoOpPodmanRuntime) CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) (*api.UnitStatus, error) {
	return nil, nil
}

func (n NoOpPodmanRuntime) StartContainer(unit api.Unit, spec *api.PodSpec, podName string) (*api.UnitStatus, error) {
	return nil, nil
}

func (n NoOpPodmanRuntime) RemoveContainer(unit *api.Unit) error {
	return nil
}

func (n NoOpPodmanRuntime) ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error) {
	return nil, nil
}

func (n NoOpPodmanRuntime) UnitRunning(unitName string) bool {
	return true
}

func (n NoOpPodmanRuntime) GetPid(unitName string) (int, bool) {
	return 0, false
}

func (n NoOpPodmanRuntime) SetPodNetwork(netNS, podIP string) {
	return
}

func NewPodmanRuntime(rootdir string) (*NoOpPodmanRuntime, error) {
	return &NoOpPodmanRuntime{}, nil
}

func GetPodmanConnection() (context.Context, error) {
	return context.TODO(), nil
}
