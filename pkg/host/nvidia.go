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

package host

import (
	"bytes"
	"os"
	"os/exec"

	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
)

const (
	NvidiaContainerCli               = "nvidia-container-cli"
	NvidiaContainerCliDownloadURL    = "https://s3.amazonaws.com/itzo-packages/nvidia-container-cli"
	NvidiaContainerCliMinimumVersion = "1.0.2"
	NvidiaContainerCliVersionFlag    = "--version"
	NvidiaSMI                        = "nvidia-smi"
)

// See https://nvidia.github.io/libnvidia-container/ on how to install
// nvidia-container-cli. We add it to our Ubuntu-based images.
func InitializeGPU(rootfs string) error {
	if _, err := os.Stat("/dev/nvidiactl"); err != nil {
		if os.IsNotExist(err) {
			// Not a GPU instance.
			return nil
		}
		glog.Errorf("Checking /dev/nvidiactl: %v", err)
		return err
	}
	cli, err := util.EnsureProg(
		NvidiaContainerCli,
		NvidiaContainerCliDownloadURL,
		NvidiaContainerCliMinimumVersion,
		NvidiaContainerCliVersionFlag,
	)
	if err != nil {
		glog.Errorf("Looking up %s: %v", NvidiaContainerCli, err)
		return err
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	// Run nvidia-smi first, since it does some initialization without which
	// nvidia-container-cli will fail.
	cmd := exec.Command(NvidiaSMI)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		glog.Errorf("Running %+v: %v stderr:\n%s", cmd, err, stderr.String())
		return err
	}
	stdout.Reset()
	stderr.Reset()
	args := []string{
		"configure",
		"--compute",
		"--no-cgroups",
		"--no-devbind",
		"--utility",
		//		"--ldconfig",
		//		"/usr/glibc-compat/sbin/ldconfig",
		rootfs,
	}
	cmd = exec.Command(cli, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		glog.Errorf("Running %+v: %v stderr:\n%s", cmd, err, stderr.String())
		return err
	}
	return nil
}
