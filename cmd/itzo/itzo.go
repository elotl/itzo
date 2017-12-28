package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/elotl/itzo/pkg/server"
	"github.com/golang/glog"
)

func main() {
	var port = flag.Int("port", 8000, "Port to listen on")
	var rootdir = flag.String("rootdir", server.DEFAULT_ROOTDIR, "Directory to install packages in")
	var approotfs = flag.String("rootfs", "", "Directory to chroot into when starting a unit")
	var appcmdline = flag.String("exec", "", "Command for starting a unit")
	// todo, ability to log to a file instead of stdout

	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")

	if *appcmdline != "" {
		err := server.StartUnit(*approotfs, strings.Split(*appcmdline, " "))
		if err != nil {
			glog.Fatalf("Error starting %s in chroot %s: %v", *appcmdline, *approotfs, err)
		} else {
			os.Exit(0)
		}
	}

	glog.Info("Starting up agent")
	server := server.New(*rootdir)
	endpoint := fmt.Sprintf("0.0.0.0:%d", *port)
	server.ListenAndServe(endpoint)
}
