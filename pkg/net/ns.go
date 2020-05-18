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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const (
	NetnsPath = "/var/run/netns"
)

type NetNamespacer interface {
	Create() error
	WithNetNamespace(cb func() error) error
	CreateVeth(ipaddr string) error
}

type OSNetNamespacer struct {
	NSName string
}

func NewOSNetNamespacer(nsname string) NetNamespacer {
	return &OSNetNamespacer{
		NSName: nsname,
	}
}

// Start a new net namespace, and ensure it persists via creating a bind mount
// to it. We use NetnsPath to ensure "ip netns" interoperability, so e.g. "ip
// netns exec <nsname> ip link ls" will work.
func (n *OSNetNamespacer) Create() error {
	os.MkdirAll(NetnsPath, 0700)
	nspath := filepath.Join(NetnsPath, n.NSName)
	f, err := os.Create(nspath)
	if err != nil {
		return err
	}
	f.Close()
	cmd := exec.Command("mount", "--bind", "/proc/self/ns/net", nspath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNET,
	}
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to start process for pod namespace: %v", err)
	}
	return nil
}

func withNetNamespace(ns netns.NsHandle, cb func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	oldNs, err := netns.GetFromPid(os.Getpid())
	if err != nil {
		return fmt.Errorf("getting old net NS: %v", err)
	}
	defer oldNs.Close()
	err = netns.Set(ns)
	if err != nil {
		return fmt.Errorf("setting new net NS: %v", err)
	}
	defer netns.Set(oldNs)
	return cb()
}

// Change to a net namespace temporarily, call a function, and switch back.
func (n *OSNetNamespacer) WithNetNamespace(cb func() error) error {
	ns, err := netns.GetFromName(n.NSName)
	if err != nil {
		return fmt.Errorf("getting net NS from %s: %v", n.NSName, err)
	}
	defer ns.Close()
	return withNetNamespace(ns, cb)
}

// Create a veth pair, and move the second one into a net namespace.
func (n *OSNetNamespacer) CreateVeth(ipaddr string) error {
	ns, err := netns.GetFromName(n.NSName)
	if err != nil {
		return err
	}
	defer ns.Close()
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: "veth0",
		},
		PeerName: "veth1",
	}
	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("can't create veth pair: %v", err)
	}
	veth0, err := netlink.LinkByName("veth0")
	if err != nil {
		return fmt.Errorf("can't find veth0: %v", err)
	}
	peer, err := netlink.LinkByName("veth1")
	if err != nil {
		return fmt.Errorf("can't find veth1: %v", err)
	}
	if err := netlink.LinkSetUp(veth); err != nil {
		return fmt.Errorf("can't bring veth0 up: %v", err)
	}
	route := &netlink.Route{
		LinkIndex: veth.Attrs().Index,
		Dst: &net.IPNet{
			IP:   net.ParseIP(ipaddr),
			Mask: net.CIDRMask(32, 32),
		},
	}
	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("can't add %q veth0 route: %v", ipaddr, err)
	}
	if err := netlink.LinkSetNsFd(peer, int(ns)); err != nil {
		return fmt.Errorf("can't move veth1 to %s: %v", n.NSName, err)
	}
	if err := withNetNamespace(ns,
		func() error {
			lo, err := netlink.LinkByName("lo")
			if err != nil {
				return fmt.Errorf(
					"can't find lo in namespace %s: %v", n.NSName, err)
			}
			if err := netlink.LinkSetUp(lo); err != nil {
				return fmt.Errorf("can't bring lo up in %s: %v", n.NSName, err)
			}
			veth1, err := netlink.LinkByName("veth1")
			if err != nil {
				return fmt.Errorf(
					"can't find veth1 in namespace %s: %v", n.NSName, err)
			}
			if err := netlink.LinkSetUp(veth1); err != nil {
				return fmt.Errorf(
					"can't bring veth1 up in %s: %v", n.NSName, err)
			}
			netaddr := &netlink.Addr{
				IPNet: &net.IPNet{
					IP:   net.ParseIP(ipaddr),
					Mask: net.CIDRMask(32, 32),
				},
			}
			if err := netlink.AddrAdd(veth1, netaddr); err != nil {
				return fmt.Errorf(
					"can't add IP address %q to veth1: %v", ipaddr, err)
			}
			route := &netlink.Route{
				LinkIndex: veth1.Attrs().Index,
				Dst: &net.IPNet{
					IP:   net.ParseIP("169.254.1.1"),
					Mask: net.CIDRMask(32, 32),
				},
				Scope: netlink.SCOPE_LINK,
			}
			if err := netlink.RouteAdd(route); err != nil {
				return fmt.Errorf("can't add 169.254.1.1 veth1 route: %v", err)
			}
			defroute := &netlink.Route{
				LinkIndex: veth1.Attrs().Index,
				Dst: &net.IPNet{
					IP:   net.ParseIP("0.0.0.0"),
					Mask: net.CIDRMask(0, 32),
				},
				Gw: net.ParseIP("169.254.1.1"),
			}
			if err := netlink.RouteAdd(defroute); err != nil {
				return fmt.Errorf("can't add default veth1 route: %v", err)
			}
			neigh := &netlink.Neigh{
				LinkIndex:    veth1.Attrs().Index,
				State:        netlink.NUD_PERMANENT,
				IP:           net.ParseIP("169.254.1.1"),
				HardwareAddr: veth0.Attrs().HardwareAddr,
			}
			if err := netlink.NeighAdd(neigh); err != nil {
				return fmt.Errorf("can't add arp veth0 entry: %v", err)
			}
			return nil
		}); err != nil {
		return err
	}
	return nil
}

type NoopNetNamespacer struct {
}

func NewNoopNetNamespacer() NetNamespacer {
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
