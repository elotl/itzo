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
