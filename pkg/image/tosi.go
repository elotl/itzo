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

package image

import (
	"fmt"

	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
)

const (
	TosiMaxRetries       = 3
	TosiOutputLimit      = 4096
	TosiExe              = "tosi"
	TosiMinimumVersion   = "v0.0.7"
	TosiURL              = "https://github.com/elotl/tosi/releases/download/v0.0.7/tosi-amd64"
	TosiUseOverlayRootfs = true
)

type Tosi struct {
	server           string
	username         string
	password         string
	image            string
	exe              string
	useOverlayRootfs bool
}

func NewTosi() *Tosi {
	return &Tosi{
		exe:              TosiExe,
		useOverlayRootfs: TosiUseOverlayRootfs,
	}
}

func (t *Tosi) SetUseOverlayRootfs(overlayRootfs bool) {
	t.useOverlayRootfs = overlayRootfs
}

func (t *Tosi) Login(server, username, password string) error {
	t.server = server
	t.username = username
	t.password = password
	return nil
}

func (t *Tosi) Pull(server, image string) error {
	t.server = server
	t.image = image
	return nil
}

func (t *Tosi) Unpack(image, dest, configPath string) error {
	if image != t.image {
		return fmt.Errorf("image mismatch %q != %q", t.image, image)
	}
	return t.run(t.server, t.image, dest, configPath, t.username, t.password)
}

func (t *Tosi) run(server, image, dest, configPath, username, password string) error {
	tp, err := util.EnsureProg(t.exe, TosiURL, TosiMinimumVersion, "-version")
	if err != nil {
		return err
	}
	if t.exe != tp {
		t.exe = tp
	}
	imageExtractFlag := "-mount"
	if !t.useOverlayRootfs {
		imageExtractFlag = "-extractto"
	}
	args := []string{
		"-image",
		image,
		imageExtractFlag,
		dest,
		"-saveconfig",
		configPath,
	}
	glog.Infof("TOSI EXEC ARGUMENTS: %s", args)
	if username != "" {
		args = append(args, []string{"-username", username}...)
	}
	if password != "" {
		args = append(args, []string{"-password", password}...)
	}
	if server != "" {
		args = append(args, []string{"-url", server}...)
	}
	return util.RunProg(tp, TosiOutputLimit, TosiMaxRetries, args...)
}
