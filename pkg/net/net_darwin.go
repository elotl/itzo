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
)

const (
	NetnsPath = "/var/run/netns"
	Veth0     = "veth0"
	Veth1     = "veth1"
)

type NetNamespacer interface {
	Create() error
	WithNetNamespace(cb func() error) error
	CreateVeth(ipaddr string) error
}

type NoopNetNamespacer struct {
}

func NewOSNetNamespacer(nsname string) NetNamespacer {
	return &NoopNetNamespacer{}
}

func (n *NoopNetNamespacer) Create() error {
	return nil
}

func (n *NoopNetNamespacer) WithNetNamespace(cb func() error) error {
	return cb()
}

func (n *NoopNetNamespacer) CreateVeth(ipaddr string) error {
	return nil
}

func NewNoopNetNamespacer() NetNamespacer {
	return &NoopNetNamespacer{}
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
	// TODO: consider if it's correct
	return mainIP, podIP, PodNetNamespaceName, nil
}

func GetPrimaryNetworkInterface() (string, error) {
	return "", fmt.Errorf("")
}