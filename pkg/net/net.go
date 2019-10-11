package net

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	sysctl "github.com/lorenzosaino/go-sysctl"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const (
	NetnsPath = "/var/run/netns"
)

// Start a new net namespace, and ensure it persists via creating a bind mount
// to it. We use NetnsPath to ensure "ip netns" interoperability, so e.g. "ip
// netns exec <nsname> ip link ls" will work.
func NewNetNamespace(nsname string) error {
	os.MkdirAll(NetnsPath, 0700)
	nspath := filepath.Join(NetnsPath, nsname)
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

// Change to a net namespace temporarily, call a function, and switch back.
func WithNetNamespace(ns netns.NsHandle, cb func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	oldNs, err := netns.Get()
	if err != nil {
		return err
	}
	defer oldNs.Close()
	err = netns.Set(ns)
	if err != nil {
		return err
	}
	defer netns.Set(oldNs)
	return cb()
}

func WithNetNamespaceFromName(nsname string, cb func() error) error {
	ns, err := netns.GetFromName(nsname)
	if err != nil {
		return err
	}
	defer ns.Close()
	return WithNetNamespace(ns, cb)
}

// Create a veth pair, and move the second one into a net namespace.
func CreateVeth(nsname, ipaddr string) error {
	ns, err := netns.GetFromName(nsname)
	if err != nil {
		return err
	}
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
		return fmt.Errorf("can't move veth1 to %s: %v", nsname, err)
	}
	if err := WithNetNamespace(ns,
		func() error {
			lo, err := netlink.LinkByName("lo")
			if err != nil {
				return fmt.Errorf(
					"can't find lo in namespace %s: %v", nsname, err)
			}
			if err := netlink.LinkSetUp(lo); err != nil {
				return fmt.Errorf("can't bring lo up in %s: %v", nsname, err)
			}
			veth1, err := netlink.LinkByName("veth1")
			if err != nil {
				return fmt.Errorf(
					"can't find veth1 in namespace %s: %v", nsname, err)
			}
			if err := netlink.LinkSetUp(veth1); err != nil {
				return fmt.Errorf("can't bring veth1 up in %s: %v", nsname, err)
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

func EnableForwarding() error {
	return sysctl.Set("net.ipv4.ip_forward", "1")
}
