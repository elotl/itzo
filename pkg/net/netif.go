// +build !darwin

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
