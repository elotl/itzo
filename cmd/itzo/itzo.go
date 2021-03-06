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

package main

import (
	"flag"
	"fmt"
	"github.com/elotl/itzo/pkg/runtime"
	"os"
	"strconv"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/server"
	"github.com/elotl/itzo/pkg/unit"
	"github.com/elotl/itzo/pkg/util"

	"github.com/golang/glog"
	quote "github.com/kballard/go-shellquote"
	"github.com/ramr/go-reaper"
)

var buildDate string

func main() {
	//  go build -ldflags "-X main.buildDate=`date -u +.%Y%m%d.%H%M%S`"
	var version = flag.Bool("version", false, "display build date")
	var disableTLS = flag.Bool("disable-tls", false, "don't use tls")
	var port = flag.Int("port", 6421, "Port to listen on")
	var rootdir = flag.String("rootdir", server.DEFAULT_ROOTDIR, "Directory to install packages in")
	var podname = flag.String("podname", "", "Pod name")
	var hostname = flag.String("hostname", "", "Pod hostname")
	var appunit = flag.String("unit", "", "Unit name")
	var appcmdline = flag.String("exec", "", "Command for starting a unit")
	var apprestartpolicy = flag.String("restartpolicy", string(api.RestartPolicyAlways), "Unit restart policy: always, never or onfailure")
	var workingdir = flag.String("workingdir", "", "Working directory for unit")
	var netns = flag.String("netns", "", "Pod network namespace name")
	// todo, ability to log to a file instead of stdout
	var usePodman = flag.Bool("use-podman", false, "use podman.io as container runtime")
	var useAnka = flag.Bool("use-anka", false, "use Veertu's anka as a VM runtime")

	flag.Set("logtostderr", "true")
	flag.Parse()

	go reaper.Reap()

	if *appcmdline != "" {
		policy := api.RestartPolicy(*apprestartpolicy)
		if *usePodman {
			glog.Errorf("Unexpected behavior, unitmanager called itzo runtime with use podman flag")
			os.Exit(1)
		}
		glog.Infof("Starting %s for pod %s unit %s; restart policy is %v",
			*appcmdline, *podname, *appunit, policy)
		cmdargs, err := quote.Split(*appcmdline)
		if err != nil {
			glog.Fatalf("Invalid command '%s' for unit %s: %v",
				*appcmdline, *appunit, err)
		}
		err = unit.StartUnit(*rootdir, *podname, *hostname, *appunit, *workingdir, *netns, cmdargs, policy)
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
	runtimeName := runtime.ItzoRuntimeName
	if *usePodman {
		runtimeName = runtime.PodmanRuntimeName
	}
	if *useAnka {
		runtimeName = runtime.AnkaRuntimeName
	}

	glog.Infof("Starting up agent, is podman used? %s", strconv.FormatBool(*usePodman))
	// TODO if podman flag is set, ensure that podman service is running
	server := server.New(*rootdir, runtimeName)
	endpoint := fmt.Sprintf("0.0.0.0:%d", *port)
	server.ListenAndServe(endpoint, *disableTLS)
}
