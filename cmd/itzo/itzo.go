package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/elotl/itzo/pkg/server"
	"github.com/golang/glog"
)

var buildDate string

func main() {
	//  go build -ldflags "-X main.buildDate=`date -u +.%Y%m%d.%H%M%S`"
	var printBuild = flag.Bool("build", false, "display build date")
	var port = flag.Int("port", 8000, "Port to listen on")
	var rootdir = flag.String("rootdir", server.DEFAULT_ROOTDIR, "Directory to install packages in")
	var approotfs = flag.String("rootfs", "", "Directory to chroot into when starting a unit")
	var appcmdline = flag.String("exec", "", "Command for starting a unit")
	var apprestartpolicy = flag.String("restartpolicy", "always", "Unit restart policy: always, never or onfailure")
	// todo, ability to log to a file instead of stdout

	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")

	if *appcmdline != "" {
		policy := server.StringToRestartPolicy(*apprestartpolicy)
		glog.Infof("Starting %s in %s; restart policy is %v",
			*appcmdline, *approotfs, policy)
		err := server.StartUnit(*approotfs, strings.Split(*appcmdline, " "), policy)
		if err != nil {
			glog.Fatalf("Error starting %s in chroot %s: %v", *appcmdline, *approotfs, err)
		} else {
			os.Exit(0)
		}
	}

	if *printBuild {
		fmt.Println("Built On:", buildDate)
		os.Exit(0)
	}

	glog.Info("Starting up agent")
	server := server.New(*rootdir)
	endpoint := fmt.Sprintf("0.0.0.0:%d", *port)
	server.ListenAndServe(endpoint)
}
