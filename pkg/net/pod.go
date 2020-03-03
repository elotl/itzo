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

package net

import (
	"fmt"

	"github.com/elotl/itzo/pkg/cloud"

	"github.com/golang/glog"
	utildbus "k8s.io/kubernetes/pkg/util/dbus"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	utilexec "k8s.io/utils/exec"
)

const (
	PodNetNamespaceName = "pod"
)

func setupPodNetwork(podIP string) error {
	nser := NewOSNetNamespacer(PodNetNamespaceName)
	execer := utilexec.New()
	dbus := utildbus.New()
	protocol := utiliptables.ProtocolIpv4
	iptInterface := utiliptables.New(execer, dbus, protocol)
	netIf, err := GetPrimaryNetworkInterface()
	if err != nil {
		glog.Errorf("retrieving pod IP address: %v", err)
		return err
	}
	err = EnsurePodMasq(iptInterface, netIf, podIP)
	if err != nil {
		glog.Errorf("looking up main network interface: %v", err)
		return err
	}
	err = EnableForwarding()
	if err != nil {
		glog.Errorf("enabling IP forwarding: %v", err)
		return err
	}
	err = nser.Create()
	if err != nil {
		glog.Errorf("creating pod network namespace: %v", err)
		return err
	}
	err = nser.CreateVeth(podIP)
	if err != nil {
		glog.Errorf("creating veth pair: %v", err)
		return err
	}
	return nil
}

func SetupNetNamespace(podIP string) (string, string, string, error) {
	cloudInfo, err := cloud.NewCloudInfo()
	if err != nil {
		return "", "", "", fmt.Errorf("creating metadata client: %v", err)
	}
	mainIP, err := cloudInfo.GetMainIPv4Address()
	if err != nil {
		glog.Errorf("unable to determine main IP address: %v", err)
		return "", "", "", err
	}
	if podIP == "" {
		return mainIP, mainIP, "", nil
	}
	err = setupPodNetwork(podIP)
	if err != nil {
		return "", "", "", fmt.Errorf("setting up pod network: %v", err)
	}
	return mainIP, podIP, PodNetNamespaceName, nil
}
