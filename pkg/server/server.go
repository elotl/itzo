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

// aws s3 cp itzo s3://itzo-download/ --acl public-read

package server

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/host"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/metrics"
	"github.com/elotl/itzo/pkg/mount"
	itzonet "github.com/elotl/itzo/pkg/net"
	"github.com/elotl/itzo/pkg/unit"
	"github.com/elotl/itzo/pkg/util"
	"github.com/elotl/wsstream"
	"github.com/golang/glog"
	"github.com/gorilla/websocket"
)

const (
	MULTIPART_PACKAGE = "package"
	ITZO_DIR          = "/tmp/itzo"
	CERTS_DIR         = ITZO_DIR
	DEFAULT_ROOTDIR   = ITZO_DIR + "/units"
	ITZO_VERSION      = "1.0"
	FILE_BYTES_LIMIT  = 4096
	// Screw it, I'm changing to go convention, no captials...
	logTailPeriod = 500 * time.Millisecond
	// We really want metrics every 15s but this allows a bit of
	// wiggle room on the timing
	minMetricPeriod = 14 * time.Second
)

// Some kind of invalid input from the user. Useful here to decide when to
// return a 4xx vs a 5xx.
type ParameterError struct {
	err error
}

func (pe *ParameterError) Error() string {
	if pe.err != nil {
		return pe.err.Error()
	}
	return ""
}

type ServerUnitMgr interface {
	GetLogBuffer(unit string) (*logbuf.LogBuffer, error)
	ReadLogBuffer(unit string, n int) ([]logbuf.LogEntry, error)
	UnitRunning(unit string) bool
	GetPid(unitName string) (int, bool)
}

type UnitManager interface {
	ServerUnitMgr
	unit.UnitRunner
}

type Server struct {
	env                 EnvStore
	httpServer          *http.Server
	mux                 http.ServeMux
	startTime           time.Time
	podController       *PodController
	unitMgr             ServerUnitMgr
	wsUpgrader          websocket.Upgrader
	installRootdir      string
	lastMetricTime      time.Time
	metrics             *metrics.Metrics
	primaryIP           string
	secondaryIP         string
	podIP               string
	podNetworkInterface string
	networkAgentCmd     *exec.Cmd
}

func New(rootdir string, usePodman bool) *Server {
	if rootdir == "" {
		rootdir = DEFAULT_ROOTDIR
	}
	mounter := mount.NewOSMounter(rootdir)

	var um UnitManager
	if usePodman {
		um = unit.NewPodmanManager()
	} else {
		um = unit.NewUnitManager(rootdir)
	}
	pc := NewPodController(rootdir, mounter, um)
	pc.Start()
	return &Server{
		env:            EnvStore{},
		startTime:      time.Now().UTC(),
		installRootdir: rootdir,
		podController:  pc,
		unitMgr:        um,
		wsUpgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		metrics:        metrics.New(),
		lastMetricTime: time.Now().Add(-minMetricPeriod),
	}
}

func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		status, initStatus, err := s.podController.GetStatus()
		if err != nil {
			serverError(w, err)
			return
		}
		var resourceUsage api.ResourceMetrics
		if time.Since(s.lastMetricTime) > minMetricPeriod {
			resourceUsage = s.metrics.GetSystemMetrics(s.podNetworkInterface)
			for _, us := range append(status, initStatus...) {
				unitResourceUsage := s.metrics.GetUnitMetrics(us.Name)
				for k, v := range unitResourceUsage {
					// Add unit resource usage to the main map. Keys are in the
					// form of "unitname.metric", e.g. "foobar.cpuUsage",
					// "foobar.memoryUsage", etc.
					resourceUsage[k] = v
				}
			}
			s.lastMetricTime = time.Now()
		}

		// Put the actual pod IP into the status reply to ensure the right IP
		// address will show up in the pod status from Milpa (i.e. if the pod
		// uses host networking, the primary IP address is the pod IP, otherwise
		// the secondary IP address and a separate network namespace is used
		// for the pod).
		reply := api.PodStatusReply{
			UnitStatuses:     status,
			InitUnitStatuses: initStatus,
			ResourceUsage:    resourceUsage,
			PodIP:            s.podIP,
		}
		buf, err := json.Marshal(&reply)
		if err != nil {
			serverError(w, err)
			return
		}
		fmt.Fprintf(w, "%s", buf)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) startNetworkAgent(IP, nodeName string) {
	cmd := host.EnsureNetworkAgent(IP, nodeName, ITZO_DIR)
	if cmd == nil {
		return
	}
	s.networkAgentCmd = cmd
	go func() {
		err := cmd.Wait()
		s.networkAgentCmd = nil
		if err != nil {
			glog.Warningf("waiting for network agent: %v", err)
		}
		glog.Infof("network agent exit code: %d", cmd.ProcessState.ExitCode())
	}()
}

func (s *Server) updateHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		var params api.PodParameters
		err := json.NewDecoder(r.Body).Decode(&params)
		if err != nil {
			badRequest(w,
				fmt.Sprintf("Error decoding pod update request: %v", err))
			return
		}
		if s.primaryIP == "" && s.secondaryIP == "" {
			primaryIP, secondaryIP, podNS, err := itzonet.SetupNetNamespace(
				params.PodIP)
			if err != nil {
				glog.Errorf("%v", err)
				serverError(w, err)
				return
			}
			s.primaryIP = primaryIP
			s.secondaryIP = secondaryIP
			s.podNetworkInterface = itzonet.Veth0
			if api.IsHostNetwork(params.Spec.SecurityContext) {
				s.podIP = s.primaryIP
				netif, err := itzonet.GetPrimaryNetworkInterface()
				if err != nil {
					glog.Warningf("getting primary network interface: %v", err)
					netif = "eth0"
				}
				s.podNetworkInterface = netif
			} else {
				s.podIP = s.secondaryIP
				s.podController.SetPodNetwork(podNS, s.podIP)
			}
			glog.Infof("IP addresses: %q %q pod network namespace: %q",
				s.primaryIP, s.podIP, podNS)
		}
		if s.networkAgentCmd == nil && params.NodeName != "" {
			s.startNetworkAgent(s.primaryIP, params.NodeName)
		}
		err = s.podController.UpdatePod(&params)
		if err != nil {
			glog.Errorf("%v", err)
			serverError(w, err)
			return
		}
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) pingHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		fmt.Fprintf(w, "pong")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		fmt.Fprintf(w, "%s", ITZO_VERSION)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) logsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// EVERYTHING IS TERRIBLE! If the request came from our
		// websocket library, the query params are in the path and
		// r.URL.String() doesn't decode them correctly (they get
		// escaped).  However, if the request came from a standard web
		// client (http.Client) the query params are already parsed
		// out into URL.RawQuery.  Lets look into the URL and see what
		// we need to parse...  Yuck!
		var parsedURL *url.URL
		var err error
		if r.URL.RawQuery != "" {
			parsedURL, err = r.URL.Parse(r.URL.String())
		} else {
			parsedURL, err = r.URL.Parse(r.URL.Path)
		}

		if err != nil {
			badRequest(w, err.Error())
			return
		}

		path := strings.TrimPrefix(parsedURL.Path, "/")
		parts := strings.Split(path, "/")
		unit := ""
		if len(parts) > 3 {
			unit = strings.Join(parts[3:], "/")
		}

		// todo, this is a bit messy here, break it out if possible
		q := parsedURL.Query()
		follow := q.Get("follow")
		withMetadata := false
		if q.Get("metadata") == "1" {
			withMetadata = true
		}
		if follow != "" {
			// Bug: if the unit gets closed or quits, we don't know
			// about the closure
			unitName, err := s.podController.GetUnitName(unit)
			if err != nil {
				badRequest(w, err.Error())
				return
			}
			logBuffer, err := s.unitMgr.GetLogBuffer(unitName)
			if err != nil {
				badRequest(w, err.Error())
				return
			}
			s.RunLogTailer(w, r, unitName, withMetadata, logBuffer)
			return
		}

		n := 0
		numBytes := 0
		lines := q.Get("lines")
		strBytes := q.Get("bytes")
		if lines != "" {
			if i, err := strconv.Atoi(lines); err == nil {
				n = i
			}
		}
		if strBytes != "" {
			if i, err := strconv.Atoi(strBytes); err == nil {
				numBytes = i
			}
		}

		unitName, err := s.podController.GetUnitName(unit)
		if err != nil {
			badRequest(w, err.Error())
			return
		}
		logs, err := s.unitMgr.ReadLogBuffer(unitName, n)
		if err != nil {
			badRequest(w, err.Error())
			return

		}
		var buffer bytes.Buffer
		for _, entry := range logs {
			buffer.WriteString(entry.Format(withMetadata))
		}
		w.Header().Set("Content-Type", "text/plain")
		buffStr := buffer.String()
		if numBytes > 0 && len(buffStr) > numBytes {
			startOffset := len(buffStr) - numBytes
			buffStr = buffStr[startOffset:]
		}
		fmt.Fprintf(w, "%s", buffStr)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) RunLogTailer(w http.ResponseWriter, r *http.Request, unitName string, withMetadata bool, logBuffer *logbuf.LogBuffer) {
	ws, err := s.doUpgrade(w, r)
	if err != nil {
		return // Do upgrade will write errors to the client
	}
	defer ws.CloseAndCleanup()

	fileTicker := time.NewTicker(logTailPeriod)
	defer fileTicker.Stop()
	lastOffset := logBuffer.GetOffset()
	var entries []logbuf.LogEntry
	for {
		select {
		case <-ws.Closed():
			return
		case <-fileTicker.C:
			unitRunning := s.unitMgr.UnitRunning(unitName)
			if !unitRunning {
				// We can finish running but still have some data left
				// in the buffer. If that's the case, go through one
				// more time and grab the last of hte data from the
				// buffer.  When there's no more data getting put in
				// the buffer, then we can exit. This is useful for CI
				// setups where we want ALL the output from the
				// subprocess.
				entries, _ = logBuffer.ReadSince(lastOffset)
				if len(entries) == 0 {
					writeWSError(ws, "Unit %s is not running\n", unitName)
					return
				}
			}
			entries, lastOffset = logBuffer.ReadSince(lastOffset)
			if len(entries) > 0 {
				msg := make([]byte, 0, 1024)
				for i := 0; i < len(entries); i++ {
					msg = append(msg, []byte(entries[i].Format(withMetadata))...)
				}
				if err := ws.WriteMsg(wsstream.StdoutChan, msg); err != nil {
					glog.Errorln("Error writing logs to buffer:", err)
					return
				}
			}
		}
	}
}

func (s *Server) fileHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		path := r.FormValue("path")
		if path == "" {
			badRequest(w, "Missing path parameter")
			return
		}
		lines := 0
		numBytes := 0
		strLines := r.FormValue("lines")
		strBytes := r.FormValue("bytes")
		if strLines != "" {
			if i, err := strconv.Atoi(strLines); err == nil {
				lines = i
			}
		}
		if strBytes != "" {
			if i, err := strconv.Atoi(strBytes); err == nil {
				numBytes = i
			}
		}
		if numBytes == 0 && lines == 0 {
			numBytes = FILE_BYTES_LIMIT
		}
		s, err := util.TailFile(path, lines, int64(numBytes))
		if err != nil {
			badRequest(w, "Error reading file "+err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "%s", s)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) resizevolumeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		if err := resizeVolume(); err != nil {
			glog.Errorln("resizing volume:", err)
			serverError(w, err)
			return
		}
	default:
		http.NotFound(w, r)
	}
}

func saveFile(r io.Reader) (filename string, n int64, err error) {
	tmpfile, err := ioutil.TempFile("", "milpa-pkg-")
	if err != nil {
		return "", 0, err
	}
	defer tmpfile.Close()
	filename = tmpfile.Name()
	written, err := io.Copy(tmpfile, r)
	if err != nil {
		os.Remove(filename)
		filename = ""
	}
	return filename, written, err
}

func (s *Server) deployHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		pod := ""
		name := ""
		if len(parts) != 5 {
			err := fmt.Errorf("invalid deploy path %s", r.URL.Path)
			glog.Errorf("%v", err)
			serverError(w, err)
			return
		}
		pod = parts[3]
		name = parts[4]
		formFile, _, err := r.FormFile(MULTIPART_PACKAGE)
		if err != nil {
			glog.Errorf("parsing form for package %s/%s deploy: %v",
				pod, name, err)
			serverError(w, err)
			return
		}
		defer formFile.Close()
		pkgfile, n, err := saveFile(formFile)
		if err != nil {
			glog.Errorf("saving file for package %s/%s deploy: %v",
				pod, name, err)
			serverError(w, err)
			return
		}
		defer os.Remove(pkgfile)
		glog.Infof("package for %s/%s saved as: %s (%d bytes)",
			pod, name, pkgfile, n)
		if err = DeployPackage(pkgfile, s.installRootdir, name); err != nil {
			glog.Errorf("deploying package %s: %v", name, err)
			serverError(w, err)
			return
		}
		glog.Infof("deployed package from file %s (%d bytes)", pkgfile, n)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) servePortForward(w http.ResponseWriter, r *http.Request) {
	ws, err := s.doUpgrade(w, r)
	if err != nil {
		return
	}
	defer ws.CloseAndCleanup()

	var params api.PortForwardParams
	err = getInitialParams(ws, &params)
	if err != nil {
		return
	}

	clientConn, err := net.Dial(
		"tcp", fmt.Sprintf("%s:%s", s.podIP, params.Port))
	if err != nil {
		writeWSError(ws, "error connecting to port %s: %v\n", params.Port, err)
		return
	}
	defer clientConn.Close()

	portWriter := bufio.NewWriter(clientConn)
	portReader := bufio.NewReader(clientConn)

	// if either side hangs up, we close the websocket connection
	// and end the interaction
	wsToPort := ws.CreateReader(wsstream.StdinChan)
	go func() {
		io.Copy(portWriter, wsToPort)
		ws.CloseAndCleanup()
	}()

	// When starting port-forward, milpa will check that there is a
	// process listening on the node by attempting a test port forward
	// command. It speeds up a timeout if we send back an empty
	// message here.  Since it's empty it shouldn't matter on the
	// other end when this gets called for real.
	ws.WriteMsg(wsstream.StdoutChan, []byte(""))
	wsFromPort := ws.CreateWriter(wsstream.StdoutChan)
	go func() {
		io.Copy(wsFromPort, portReader)
		ws.CloseAndCleanup()
	}()

	ws.RunDispatch()
}

func (s *Server) runAttach(ws *wsstream.WSReadWriter, params api.AttachParams) {
	unitName, err := s.podController.GetUnitName(params.UnitName)
	if err != nil {
		writeWSError(ws, err.Error())
		return
	}
	_, exists := s.unitMgr.GetPid(unitName)
	if !exists {
		writeWSError(ws, "Could not find running process for unit named %s\n", unitName)
		return
	}

	logBuffer, err := s.unitMgr.GetLogBuffer(unitName)
	if err != nil {
		writeWSError(ws, err.Error())
		return
	}

	if params.Interactive {
		u, err := unit.OpenUnit(s.installRootdir, unitName)
		if err != nil {
			msg := fmt.Sprintf("Could not open unit %s: %v\n", unitName, err)
			writeWSError(ws, msg)
			return
		}
		inWriter, err := u.OpenStdinWriter()
		if err != nil {
			msg := fmt.Sprintf("Could not open stdin for unit %s: %v\n", unitName, err)
			writeWSError(ws, msg)
			return
		}
		wsStdinReader := ws.CreateReader(wsstream.StdinChan)
		go ws.RunDispatch()
		go io.Copy(inWriter, wsStdinReader)
	}

	// copy our stdout and stderr (from logbuffer) to the websocket
	fileTicker := time.NewTicker(logTailPeriod)
	defer fileTicker.Stop()
	lastOffset := logBuffer.GetOffset()
	var entries []logbuf.LogEntry
	for {
		select {
		case <-ws.Closed():
			return
		case <-fileTicker.C:
			unitRunning := s.unitMgr.UnitRunning(unitName)
			if !unitRunning {
				writeWSError(ws, "Unit %s is not running\n", unitName)
				return
			}

			entries, lastOffset = logBuffer.ReadSince(lastOffset)
			for i := 0; i < len(entries); i++ {
				if entries[i].Source == logbuf.HelperLogSource {
					continue
				}
				channel := wsstream.StdoutChan
				if entries[i].Source == logbuf.StderrLogSource {
					channel = wsstream.StderrChan
				}
				err := ws.WriteMsg(channel, []byte(entries[i].Line))
				if err != nil {
					glog.Errorln("Error writing output to websocket", err)
					return
				}
			}
		}
	}
}

func (s *Server) serveAttach(w http.ResponseWriter, r *http.Request) {
	ws, err := s.doUpgrade(w, r)
	if err != nil {
		return
	}
	defer ws.CloseAndCleanup()

	var params api.AttachParams
	err = getInitialParams(ws, &params)
	if err != nil {
		return
	}

	s.runAttach(ws, params)
}

func (s *Server) serveExec(w http.ResponseWriter, r *http.Request) {
	ws, err := s.doUpgrade(w, r)
	if err != nil {
		glog.Errorf("upgrading WS connection for exec: %v", err)
		return
	}
	defer ws.CloseAndCleanup()

	var params api.ExecParams
	err = getInitialParams(ws, &params)
	if err != nil {
		glog.Errorf("getting initial parameters for exec: %v", err)
		return
	}

	s.runExec(ws, params)
}

func (s *Server) runcmdHandler(w http.ResponseWriter, r *http.Request) {
	var params api.RunCmdParams
	err := json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		badRequest(w, fmt.Sprintf("Error decoding run command request: %v", err))
		return
	}
	if len(params.Command) == 0 {
		badRequest(w, fmt.Sprintf("Empty command argument"))
		return

	}
	cmd := exec.Command(params.Command[0], params.Command[1:]...)
	output, err := cmd.Output()
	if err != nil {
		serverError(w, fmt.Errorf("running command %s: %v", err, params.Command[0]))
		return
	}
	// Todo: do we need to base64 encode the output from the command?
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, string(output))
}

func (s *Server) getHandlers() {
	s.mux = http.ServeMux{}
	s.mux.HandleFunc("/rest/v1/deploy/", s.deployHandler)
	s.mux.HandleFunc("/rest/v1/logs/", s.logsHandler)
	s.mux.HandleFunc("/rest/v1/file/", s.fileHandler)
	s.mux.HandleFunc("/rest/v1/runcmd/", s.runcmdHandler)
	// The updatepod endpoint is used to send in a full PodParameters struct.
	s.mux.HandleFunc("/rest/v1/updatepod", s.updateHandler)
	// This endpoint gives back the status of the whole pod.
	s.mux.HandleFunc("/rest/v1/status", s.statusHandler)
	s.mux.HandleFunc("/rest/v1/resizevolume", s.resizevolumeHandler)
	s.mux.HandleFunc("/rest/v1/ping", s.pingHandler)
	s.mux.HandleFunc("/rest/v1/version", s.versionHandler)

	// streaming endpoints
	s.mux.HandleFunc("/rest/v1/portforward/", s.servePortForward)
	s.mux.HandleFunc("/rest/v1/attach/", s.serveAttach)
	s.mux.HandleFunc("/rest/v1/exec/", s.serveExec)

}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe(addr string, disableTLS bool) {
	s.getHandlers()

	if disableTLS {
		s.httpServer = &http.Server{Addr: addr, Handler: s}
		glog.Fatalln(s.httpServer.ListenAndServe())
		return
	}

	caCert, err := ioutil.ReadFile(filepath.Join(CERTS_DIR, "ca.crt"))
	if err != nil {
		glog.Fatalln("Could not load root cert", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig := &tls.Config{
		ClientCAs:  caCertPool,
		ServerName: "MilpaNode",
		ClientAuth: tls.RequireAndVerifyClientCert,
	}

	tlsConfig.BuildNameToCertificate()
	s.httpServer = &http.Server{
		Addr:      addr,
		Handler:   s,
		TLSConfig: tlsConfig,
	}

	glog.Fatalln(s.httpServer.ListenAndServeTLS(
		filepath.Join(CERTS_DIR, "server.crt"),
		filepath.Join(CERTS_DIR, "server.key")))
}
