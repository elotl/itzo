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

package cloud

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
)

const (
	metadataLocalIP = "local-ipv4"
	metadataMACPath = "network/interfaces/macs/"
	metadataIPv4s   = "/local-ipv4s"
)

type AwsCloudInfo struct {
	metadata *ec2metadata.EC2Metadata
}

func NewAwsCloudInfo() (CloudInfo, error) {
	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}
	metadata := ec2metadata.New(sess)
	if !metadata.Available() {
		return nil, fmt.Errorf("unable to access EC2 metadata service")
	}
	return &AwsCloudInfo{
		metadata: metadata,
	}, nil
}

// Get the IP address assigned to the pod. We'll need something similar to this
// for each cloud.
func (a *AwsCloudInfo) GetPodIPv4Address() (string, error) {
	address, err := a.metadata.GetMetadata(metadataLocalIP)
	if err != nil {
		return "", err
	}
	macs, err := a.metadata.GetMetadata(metadataMACPath)
	if err != nil {
		return "", err
	}
	maclist := strings.Fields(macs)
	if len(maclist) < 1 {
		return "", fmt.Errorf("unable to find instance MAC address")
	}
	mac := maclist[0]
	addresses, err := a.metadata.GetMetadata(metadataMACPath + mac + metadataIPv4s)
	if err != nil {
		return "", err
	}
	addresslist := strings.Fields(addresses)
	for _, addr := range addresslist {
		if addr == address {
			// Primary IPv4 address, reserved for management.
			continue
		}
		return addr, nil
	}
	return "", fmt.Errorf("unable to find pod IP, addresses: %v", addresses)
}

// Get the main IP address used by itzo.
func (a *AwsCloudInfo) GetMainIPv4Address() (string, error) {
	address, err := a.metadata.GetMetadata(metadataLocalIP)
	if err != nil {
		return "", err
	}
	return address, nil
}
