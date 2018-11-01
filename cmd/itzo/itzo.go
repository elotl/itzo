package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/server"
	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
	quote "github.com/kballard/go-shellquote"
)

var buildDate string

func main() {
	//  go build -ldflags "-X main.buildDate=`date -u +.%Y%m%d.%H%M%S`"
	var version = flag.Bool("version", false, "display build date")
	var disableTLS = flag.Bool("disable-tls", false, "don't use tls")
	var port = flag.Int("port", 6421, "Port to listen on")
	var rootdir = flag.String("rootdir", server.DEFAULT_ROOTDIR, "Directory to install packages in")
	var appunit = flag.String("unit", "", "Unit name")
	var appcmdline = flag.String("exec", "", "Command for starting a unit")
	var apprestartpolicy = flag.String("restartpolicy", string(api.RestartPolicyAlways), "Unit restart policy: always, never or onfailure")
	var workingdir = flag.String("workingdir", "", "Working directory for unit")
	// todo, ability to log to a file instead of stdout

	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")
	if *appcmdline != "" {
		policy := api.RestartPolicy(*apprestartpolicy)
		glog.Infof("Starting %s for %s; restart policy is %v",
			*appcmdline, *appunit, policy)
		cmdargs, err := quote.Split(*appcmdline)
		if err != nil {
			glog.Fatalf("Invalid command '%s' for unit %s: %v",
				*appcmdline, *appunit, err)
		}
		err = server.StartUnit(*rootdir, *appunit, *workingdir, cmdargs, policy)
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

	glog.Info("Starting up agent")
	server := server.New(*rootdir)
	endpoint := fmt.Sprintf("0.0.0.0:%d", *port)
	server.ListenAndServe(endpoint, *disableTLS)
}
