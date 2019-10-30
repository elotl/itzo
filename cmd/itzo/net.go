package main

import (
	"github.com/elotl/itzo/pkg/cloud"
	"github.com/elotl/itzo/pkg/net"

	"github.com/golang/glog"
	utildbus "k8s.io/kubernetes/pkg/util/dbus"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	utilexec "k8s.io/utils/exec"
)

const (
	PodNetNamespaceName = "pod"
)

func setupPodNetwork(cloudInfo cloud.CloudInfo) string {
	nser := net.NewOSNetNamespacer(PodNetNamespaceName)
	glog.Infof("ensuring iptables NAT for pod IP")
	podAddr, err := cloudInfo.GetPodIPv4Address()
	if err != nil {
		glog.Warningf("failed to retrieve pod IP address: %v", err)
		return ""
	}
	glog.Infof("pod IPv4 address: %s", podAddr)
	execer := utilexec.New()
	dbus := utildbus.New()
	protocol := utiliptables.ProtocolIpv4
	iptInterface := utiliptables.New(execer, dbus, protocol)
	netIf, err := net.GetPrimaryNetworkInterface()
	if err != nil {
		glog.Warningf("failed to retrieve pod IP address: %v", err)
		return ""
	}
	glog.Infof("main network interface: %s", netIf)
	err = net.EnsurePodMasq(iptInterface, netIf, podAddr)
	if err != nil {
		glog.Warningf("failed to retrieve pod IP address: %v", err)
		return ""
	}
	glog.Infof("enabling IP forwarding")
	err = net.EnableForwarding()
	if err != nil {
		glog.Warningf("failed to enable IP forwarding: %v", err)
		return ""
	}
	glog.Infof("setting up pod network namespace")
	err = nser.Create()
	if err != nil {
		glog.Warningf("failed to create pod network namespace: %v", err)
		return ""
	}
	glog.Infof("creating pod network interfaces")
	err = nser.CreateVeth(podAddr)
	if err != nil {
		glog.Warningf("failed to set up pod network: %v", err)
		return ""
	}
	glog.Infof("created pod network interfaces and routes")
	return podAddr
}

func setupNetNamespace() (string, string, string) {
	cloudInfo, err := cloud.NewCloudInfo()
	if err != nil {
		glog.Fatalf("unable to create cloud metadata client: %v", err)
	}
	podNetNS := ""
	mainIP, err := cloudInfo.GetMainIPv4Address()
	if err != nil {
		glog.Fatalf("Unable to determine main IP address: %v", err)
	}
	podIP := mainIP
	secondaryIP := setupPodNetwork(cloudInfo)
	if secondaryIP != "" {
		podIP = secondaryIP
		podNetNS = PodNetNamespaceName
	}
	return mainIP, podIP, podNetNS
}
