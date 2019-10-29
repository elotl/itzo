package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/net"
	"github.com/elotl/itzo/pkg/server"
	"github.com/elotl/itzo/pkg/util"

	"github.com/golang/glog"
	quote "github.com/kballard/go-shellquote"
	utildbus "k8s.io/kubernetes/pkg/util/dbus"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	utilexec "k8s.io/utils/exec"
)

const (
	PodNetNamespaceName = "pod"
)

var buildDate string

func setupPodNetwork() string {
	nser := net.NewOSNetNamespacer(PodNetNamespaceName)
	glog.Infof("ensuring iptables NAT for pod IP")
	podAddr, err := util.GetPodIPv4Address()
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

func main() {
	//  go build -ldflags "-X main.buildDate=`date -u +.%Y%m%d.%H%M%S`"
	var version = flag.Bool("version", false, "display build date")
	var disableTLS = flag.Bool("disable-tls", false, "don't use tls")
	var enablePodNetworkNamespace = flag.Bool("enable-pod-network-namespace", true,
		"set up a network namespace for pod")
	var port = flag.Int("port", 6421, "Port to listen on")
	var rootdir = flag.String("rootdir", server.DEFAULT_ROOTDIR, "Directory to install packages in")
	var podname = flag.String("podname", "", "Pod name")
	var appunit = flag.String("unit", "", "Unit name")
	var appcmdline = flag.String("exec", "", "Command for starting a unit")
	var apprestartpolicy = flag.String("restartpolicy", string(api.RestartPolicyAlways), "Unit restart policy: always, never or onfailure")
	var workingdir = flag.String("workingdir", "", "Working directory for unit")
	var netns = flag.String("netns", "", "Pod network namespace name")
	// todo, ability to log to a file instead of stdout

	flag.Set("logtostderr", "true")
	flag.Parse()

	if *appcmdline != "" {
		policy := api.RestartPolicy(*apprestartpolicy)
		glog.Infof("Starting %s for pod %s unit %s; restart policy is %v",
			*appcmdline, *podname, *appunit, policy)
		cmdargs, err := quote.Split(*appcmdline)
		if err != nil {
			glog.Fatalf("Invalid command '%s' for unit %s: %v",
				*appcmdline, *appunit, err)
		}
		err = server.StartUnit(*rootdir, *podname, *appunit, *workingdir, *netns, cmdargs, policy)
		if err != nil {
			glog.Fatalf("Error starting %s for unit %s: %v",
				*appcmdline, *appunit, err)
		} else {
			os.Exit(0)
		}
	}

	if *version {
		fmt.Println("itzo version:", util.Version())
		os.Exit(0)
	}

	mainIP, err := util.GetMainIPv4Address()
	if err != nil {
		glog.Fatalf("Unable to determine main IP address: %v", err)
	}
	podNetNS := ""
	podIP := mainIP
	if *enablePodNetworkNamespace {
		secondaryIP := setupPodNetwork()
		if secondaryIP != "" {
			podIP = secondaryIP
			podNetNS = PodNetNamespaceName
		}
	}

	glog.Info("Starting up agent")
	server := server.New(*rootdir, mainIP, podIP, podNetNS)
	endpoint := fmt.Sprintf("0.0.0.0:%d", *port)
	server.ListenAndServe(endpoint, *disableTLS)
}
