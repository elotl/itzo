package net

import (
	"fmt"
	"net"
)

func GetPrimaryNetworkInterface() (string, error) {
	nics, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("Could not get interfaces: %v", err)
	}
	for _, nic := range nics {
		if nic.Flags&net.FlagUp != 1 ||
			nic.Flags&net.FlagLoopback != 0 ||
			nic.Flags&net.FlagPointToPoint != 0 {
			continue
		}
		addrs, err := nic.Addrs()
		if err != nil {
			return "", fmt.Errorf("Getting IP addresses from %q: %v",
				nic.Name, err)
		}
		if addrs == nil {
			continue
		}
		for _, addr := range addrs {
			ip := net.ParseIP(addr.String())
			if ip.IsLoopback() ||
				ip.IsMulticast() ||
				ip.IsUnspecified() {
				continue
			}
			return nic.Name, nil
		}
	}
	return "", fmt.Errorf("No usable network interface found")
}
