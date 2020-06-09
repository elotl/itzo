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
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
)

const (
	KubeRouterProg           = "kube-router"
	KubeRouterURL            = "https://milpa-builds.s3.amazonaws.com/kube-router"
	KubeRouterMinimumVersion = "v0.3.1"
)

func EnsureNetworkAgent(IP, nodeName, baseDir string) *exec.Cmd {
	pth, err := util.EnsureProg(
		KubeRouterProg, KubeRouterURL, KubeRouterMinimumVersion, "--version")
	if err != nil {
		glog.Errorf("failed to look up path of %q: %v", KubeRouterProg, err)
		return nil
	}
	// Kubeconfig has been deployed as a package. Find the actual config file
	// inside the package directory.
	kubeconfig := ""
	kubeconfigDir := path.Join(baseDir, "packages/kubeconfig")
	err = filepath.Walk(
		kubeconfigDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if info.Name() == "kubeconfig" {
				kubeconfig = path
			}
			return nil
		},
	)
	if err != nil {
		glog.Errorf("searching for kubeconfig package: %v", err)
		return nil
	}
	if kubeconfig == "" {
		glog.Errorf("no kubeconfig found")
		return nil
	}
	err = os.MkdirAll("/var/log", 0755)
	if err != nil {
		glog.Warningf("ensuring /var/log exists: %v", err)
	}
	logfile, err := os.OpenFile(
		"/var/log/kube-router.log", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		glog.Warningf("opening kube-router logfile: %v", err)
	}
	if logfile != nil {
		defer logfile.Close()
	}
	cmd := exec.Command(
		pth,
		"--kubeconfig="+kubeconfig,
		"--hostname-override="+nodeName,
		"--ip-address-override="+IP,
		"--hairpin-mode=true",
		"--disable-source-dest-check=false",
		"--enable-pod-egress=false",
		"--enable-cni=false",
		"--run-router=false",
		"--v=2",
	)
	cmd.Stdout = logfile
	cmd.Stderr = logfile
	err = cmd.Start()
	if err != nil {
		glog.Errorf("starting %v: %v", cmd, err)
		return nil
	}
	glog.Infof("%v started", cmd)
	return cmd
}
