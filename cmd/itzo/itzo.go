package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/elotl/itzo/pkg/server"
	"github.com/golang/glog"
	"github.com/google/shlex"
)

var buildDate string

func main() {
	//  go build -ldflags "-X main.buildDate=`date -u +.%Y%m%d.%H%M%S`"
	var printBuild = flag.Bool("build", false, "display build date")
	var port = flag.Int("port", 8000, "Port to listen on")
	var rootdir = flag.String("rootdir", server.DEFAULT_ROOTDIR, "Directory to install packages in")
	var appunit = flag.String("unit", "", "Unit name")
	var appcmdline = flag.String("exec", "", "Command for starting a unit")
	var apprestartpolicy = flag.String("restartpolicy", "always", "Unit restart policy: always, never or onfailure")
	// todo, ability to log to a file instead of stdout

	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")

	if *appcmdline != "" {
		policy := server.StringToRestartPolicy(*apprestartpolicy)
		glog.Infof("Starting %s for %s; restart policy is %v",
			*appcmdline, *appunit, policy)
		cmdargs, err := shlex.Split(*appcmdline)
		if err != nil {
			glog.Fatalf("Invalid command '%s' for unit %s: %v",
				*appcmdline, *appunit, err)
		}
		err = server.StartUnit(*rootdir, *appunit, cmdargs, policy)
		if err != nil {
			glog.Fatalf("Error starting %s for unit %s: %v",
				*appcmdline, *appunit, err)
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
