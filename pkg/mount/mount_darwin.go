/* Copyright 2020 Elotl Inc

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

package mount

import (
	"github.com/elotl/itzo/pkg/api"
)

type (
	mountFunc   func(source string, target string, fstype string, flags uintptr, data string) error
	unmountFunc func(target string, flags int) error
	pivoterFunc func(rootfs, oldrootfs string) error
)

type Mounter interface {
	CreateMount(volume *api.Volume) error
	DeleteMount(volume *api.Volume) error
	AttachMount(unitname, src, dst string) error
	DetachMount(unitname, dst string) error
	MountSpecial(unitname string) error
	UnmountSpecial(unitname string)
	BindMount(src, dst string) error
	Unmount(dir string) error
	PivotRoot(rootfs, oldrootfs string) error
}

type OSMounter struct {
	basedir string
}

type Mount struct {
	Source string
	Target string
	Fs     string
	Flags  int
	Data   string
}

func NewOSMounter(basedir string) Mounter {
	return &OSMounter{
		basedir: basedir,
	}
}

func (om *OSMounter) UnmountSpecial(unitname string) {
	return
}

func (om *OSMounter) MountSpecial(unitname string) error {
	return nil
}

func (om *OSMounter) BindMount(src, dst string) error {
	return nil
}

func (om *OSMounter) Unmount(dir string) error {
	return nil
}

func (om *OSMounter) PivotRoot(rootfs, oldrootfs string) error {
	return nil
}

func (om *OSMounter) CreateMount(volume *api.Volume) error {
	return nil
}

func (om *OSMounter) DeleteMount(volume *api.Volume) error {
	return nil
}

func ShareMount(target string, flags uintptr) error {
	return nil
}

func (om *OSMounter) AttachMount(unit, src, dst string) error {
	return nil
}

func (om *OSMounter) DetachMount(unit, dst string) error {
	return nil
}
