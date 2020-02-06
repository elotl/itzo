/*
Copyright 2014 The Kubernetes Authors.

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

package prober

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/util"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/probe"
	execprobe "k8s.io/kubernetes/pkg/probe/exec"
	httprobe "k8s.io/kubernetes/pkg/probe/http"
	tcprobe "k8s.io/kubernetes/pkg/probe/tcp"
	kubeexec "k8s.io/utils/exec"
)

const maxProbeRetries = 3

// Prober helps to check the liveness/readiness of a container.
type prober struct {
	unitName string
	unitEnv  []api.EnvVar
	exec     execprobe.Prober
	// probe types needs different httprobe instances so they don't
	// share a connection pool which can cause collsions to the
	// same host:port and transient failures. See #49740.
	readinessHttp httprobe.Prober
	livenessHttp  httprobe.Prober
	tcp           tcprobe.Prober
	// Runner allows us to easier test exec probes
	runner CommandRunner
}

// NewProber creates a Prober, it takes a command runner and
// several container info managers.
func newProber(
	unitName string,
	env []api.EnvVar,
) *prober {
	const followNonLocalRedirects = false
	return &prober{
		unitName:      unitName,
		unitEnv:       env,
		exec:          execprobe.New(),
		readinessHttp: httprobe.New(followNonLocalRedirects),
		livenessHttp:  httprobe.New(followNonLocalRedirects),
		tcp:           tcprobe.New(),
		runner:        &ExecRunner{},
	}
}

// probe probes the container.
func (pb *prober) probe(probeType ProbeType, probeSpec *api.Probe) (Result, error) {
	if probeSpec == nil {
		glog.Warningf("%s probe for %s is nil", probeType, pb.unitName)
		return Success, nil
	}

	result, output, err := pb.runProbeWithRetries(probeType, probeSpec, maxProbeRetries)
	if err != nil || (result != probe.Success && result != probe.Warning) {
		if err != nil {
			glog.V(1).Infof("%s probe for %q errored: %v", probeType, pb.unitName, err)
		} else { // result != probe.Success
			glog.V(1).Infof("%s probe for %q failed (%v): %s", probeType, pb.unitName, result, output)
		}
		return Failure, err
	}
	if result == probe.Warning {
		glog.V(3).Infof("%s probe for %q succeeded with a warning: %s", probeType, pb.unitName, output)
	} else {
		glog.V(3).Infof("%s probe for %q succeeded", probeType, pb.unitName)
	}
	return Success, nil
}

// runProbeWithRetries tries to probe the container in a finite loop, it returns the last result
// if it never succeeds.
func (pb *prober) runProbeWithRetries(probeType ProbeType, p *api.Probe, retries int) (probe.Result, string, error) {
	var err error
	var result probe.Result
	var output string
	for i := 0; i < retries; i++ {
		result, output, err = pb.runProbe(probeType, p)
		if err == nil {
			return result, output, nil
		}
	}
	return result, output, err
}

// buildHeaderMap takes a list of HTTPHeader <name, value> string
// pairs and returns a populated string->[]string http.Header map.
func buildHeader(headerList []api.HTTPHeader) http.Header {
	headers := make(http.Header)
	for _, header := range headerList {
		headers[header.Name] = append(headers[header.Name], header.Value)
	}
	return headers
}

func (pb *prober) runProbe(probeType ProbeType, p *api.Probe) (probe.Result, string, error) {
	timeout := time.Duration(p.TimeoutSeconds) * time.Second
	if p.Exec != nil {
		glog.V(4).Infof("Exec-Probe Unit: %v, Command: %v", pb.unitName, p.Exec.Command)
		command := util.ExpandContainerCommandOnlyStatic(p.Exec.Command, pb.unitEnv)
		return pb.exec.Probe(pb.newExecCmd(command, timeout))
	}
	if p.HTTPGet != nil {
		scheme := strings.ToLower(string(p.HTTPGet.Scheme))
		host := p.HTTPGet.Host
		if host == "" {
			host = "localhost"
		}
		port, err := extractPort(p.HTTPGet.Port)
		if err != nil {
			return probe.Unknown, "", err
		}
		path := p.HTTPGet.Path
		glog.V(4).Infof("HTTP-Probe Host: %v://%v, Port: %v, Path: %v", scheme, host, port, path)
		url := formatURL(scheme, host, port, path)
		headers := buildHeader(p.HTTPGet.HTTPHeaders)
		glog.V(4).Infof("HTTP-Probe Headers: %v", headers)
		if probeType == Liveness {
			return pb.livenessHttp.Probe(url, headers, timeout)
		} else { // readiness
			return pb.readinessHttp.Probe(url, headers, timeout)
		}
	}
	if p.TCPSocket != nil {
		port, err := extractPort(p.TCPSocket.Port)
		if err != nil {
			return probe.Unknown, "", err
		}
		host := p.TCPSocket.Host
		if host == "" {
			host = "localhost"
		}
		glog.V(4).Infof("TCP-Probe Host: %v, Port: %v, Timeout: %v", host, port, timeout)
		return pb.tcp.Probe(host, port, timeout)
	}
	glog.Warningf("Failed to find probe builder for container: %v", pb.unitName)
	return probe.Unknown, "", fmt.Errorf("Missing probe handler for %s", pb.unitName)
}

func extractPort(param intstr.IntOrString) (int, error) {
	switch param.Type {
	case intstr.String:
		return -1, fmt.Errorf("could not find port named %s in unit", param.StrVal)
	case intstr.Int:
		port := param.IntValue()
		if port > 0 && port < 65536 {
			return port, nil
		}
	default:
		return -1, fmt.Errorf("intOrString had no kind: %+v", param)
	}
	return -1, fmt.Errorf("invalid port number: %v", param)
}

// formatURL formats a URL from args.  For testability.
func formatURL(scheme string, host string, port int, path string) *url.URL {
	u, err := url.Parse(path)
	// Something is busted with the path, but it's too late to reject it. Pass it along as is.
	if err != nil {
		u = &url.URL{
			Path: path,
		}
	}
	u.Scheme = scheme
	u.Host = net.JoinHostPort(host, strconv.Itoa(port))
	return u
}

// execCmd implements k8s exec.Cmd interface. This allows us to
// use k8s.io/kubernetes/pkg/probe/exec package without changes
type execCmd struct {
	// run executes a command in the current namespace. Combined
	// stdout and stderr output is always returned. An error is
	// returned if one occurred.
	run    func() ([]byte, error)
	writer io.Writer
}

func (pb *prober) newExecCmd(cmd []string, timeout time.Duration) kubeexec.Cmd {
	return execCmd{run: func() ([]byte, error) {
		return pb.runner.RunWithTimeout(cmd, timeout)
	}}
}

func (eic execCmd) Run() error {
	return fmt.Errorf("unimplemented")
}

// Here we convert os/exec.ExitError so that the
// k8s.io/kubernetes/pkg/probe/exec package can interpret the output
// correctly.
func (eic execCmd) CombinedOutput() ([]byte, error) {
	output, err := eic.run()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			err = kubeexec.ExitErrorWrapper{e}
		}
	}
	return output, err
}

func (eic execCmd) Output() ([]byte, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (eic execCmd) SetDir(dir string) {
	//unimplemented
}

func (eic execCmd) SetStdin(in io.Reader) {
	//unimplemented
}

func (eic execCmd) SetStdout(out io.Writer) {
	eic.writer = out
}

func (eic execCmd) SetStderr(out io.Writer) {
	eic.writer = out
}

func (eic execCmd) SetEnv(env []string) {
	//unimplemented
}

func (eic execCmd) Stop() {
	//unimplemented
}

func (eic execCmd) Start() error {
	data, err := eic.CombinedOutput()
	if eic.writer != nil {
		eic.writer.Write(data)
	}
	return err
}

func (eic execCmd) Wait() error {
	return nil
}

func (eic execCmd) StdoutPipe() (io.ReadCloser, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (eic execCmd) StderrPipe() (io.ReadCloser, error) {
	return nil, fmt.Errorf("unimplemented")
}
