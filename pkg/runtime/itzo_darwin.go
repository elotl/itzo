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
)

type ImagePuller struct {}

func (i ImagePuller) PullImage(rootdir, name, image string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) error {
	return nil
}

type ItzoRuntime struct {}

func (i ItzoRuntime) RunPodSandbox(spec *api.PodSpec) error {
	panic("implement me")
}

func (i ItzoRuntime) StopPodSandbox(spec *api.PodSpec) error {
	panic("implement me")
}

func (i ItzoRuntime) RemovePodSandbox(spec *api.PodSpec) error {
	panic("implement me")
}

func (i ItzoRuntime) CreateContainer(unit api.Unit, spec *api.PodSpec, podName string, registryCredentials map[string]api.RegistryCredentials, useOverlayfs bool) (*api.UnitStatus, error) {
	panic("implement me")
}

func (i ItzoRuntime) StartContainer(unit api.Unit, spec *api.PodSpec, podName string) (*api.UnitStatus, error) {
	panic("implement me")
}

func (i ItzoRuntime) RemoveContainer(unit *api.Unit) error {
	panic("implement me")
}

func (i ItzoRuntime) ContainerStatus(unitName, unitImage string) (*api.UnitStatus, error) {
	panic("implement me")
}

func (i ItzoRuntime) GetLogBuffer(options LogOptions) (*logbuf.LogBuffer, error) {
	panic("implement me")
}

func (i ItzoRuntime) UnitRunning(unitName string) bool {
	panic("implement me")
}

func (i ItzoRuntime) GetPid(unitName string) (int, bool) {
	panic("implement me")
}

func (i ItzoRuntime) SetPodNetwork(netNS, podIP string) {
	panic("implement me")
}

func (i ItzoRuntime) ReadSystemMetrics(s string) api.ResourceMetrics {
	panic("implement me")
}

func (i ItzoRuntime) ReadUnitMetrics(s string) api.ResourceMetrics {
	panic("implement me")
}

func NewItzoRuntime(rootdir string, unitMgr UnitRunner, mounter Mounter, imgPuller ImageService) *ItzoRuntime {
	return &ItzoRuntime{}
}

