package util

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

// Get the IP address assigned to the pod. We'll need something similar to this
// for each cloud.
func GetPodIPv4Address() (string, error) {
	sess, err := session.NewSession()
	if err != nil {
		return "", err
	}
	metadata := ec2metadata.New(sess)
	address, err := metadata.GetMetadata(metadataLocalIP)
	if err != nil {
		return "", err
	}
	macs, err := metadata.GetMetadata(metadataMACPath)
	if err != nil {
		return "", err
	}
	maclist := strings.Fields(macs)
	if len(maclist) < 1 {
		return "", fmt.Errorf("unable to find instance MAC address")
	}
	mac := maclist[0]
	addresses, err := metadata.GetMetadata(metadataMACPath + mac + metadataIPv4s)
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
func GetMainIPv4Address() (string, error) {
	sess, err := session.NewSession()
	if err != nil {
		return "", err
	}
	metadata := ec2metadata.New(sess)
	address, err := metadata.GetMetadata(metadataLocalIP)
	if err != nil {
		return "", err
	}
	return address, nil
}
