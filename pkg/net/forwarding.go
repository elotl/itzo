package net

import sysctl "github.com/lorenzosaino/go-sysctl"

func EnableForwarding() error {
	return sysctl.Set("net.ipv4.ip_forward", "1")
}
