package main

import (
	"flag"
	"fmt"

	"github.com/elotl/itzo/pkg/server"
	"github.com/golang/glog"
)

func main() {
	var port = flag.Int("port", 8000, "Port to listen on")
	var rootdir = flag.String("installrootdir", "/tmp/milpa-pkg", "Directory to install packages in")
	// todo, ability to log to a file instead of stdout

	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")
	glog.Info("Starting up agent")
	server := server.New()
	endpoint := fmt.Sprintf("0.0.0.0:%d", *port)
	server.ListenAndServe(endpoint, *rootdir)
}
