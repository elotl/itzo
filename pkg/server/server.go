// aws s3 cp itzo s3://itzo-download/ --acl public-read

package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

const (
	MULTIPART_FILE_NAME = "file"
	MULTIPART_PKG_NAME  = "pkg"
	CERTS_DIR           = "/tmp/milpa"
	DEFAULT_ROOTDIR     = "/tmp/milpa/units"
	ITZO_VERSION        = "1.0"
	FILE_BYTES_LIMIT    = 4096
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

type Server struct {
	env        EnvStore
	httpServer *http.Server
	mux        http.ServeMux
	startTime  time.Time

	// Packages will be installed under this directory (created if it does not
	// exist).
	installRootdir string
}

func New(rootdir string) *Server {
	if rootdir == "" {
		rootdir = DEFAULT_ROOTDIR
	}
	return &Server{
		env:            EnvStore{},
		startTime:      time.Now().UTC(),
		installRootdir: rootdir,
	}
}

func serverError(w http.ResponseWriter, err error) {
	msg := fmt.Sprintf("500 Server Error: %s", err.Error())
	http.Error(w, msg, http.StatusInternalServerError)
}

func badRequest(w http.ResponseWriter, errMsg string) {
	msg := fmt.Sprintf("400 Bad Request: %s", errMsg)
	http.Error(w, msg, http.StatusBadRequest)
}

func (s *Server) makeAppEnv(unit string) []string {
	// I don't think we should pull in the environ from itzo...
	//e := os.Environ()
	e := []string{}
	for _, d := range s.env.Items(unit) {
		e = append(e, fmt.Sprintf("%s=%s", d[0], d[1]))
	}
	return e
}

func getURLPart(i int, path string) (string, error) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if i < 1 || i > len(parts) {
		return "", fmt.Errorf("Could not find part %d of url", i)
	}
	return parts[i-1], nil
}

func (s *Server) startHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "PUT":
		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, "appHandler ParseForm() err: %v", err)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		unit := ""
		if len(parts) > 3 {
			unit = strings.Join(parts[3:], "/")
		}
		command := r.FormValue("command")
		if command == "" {
			badRequest(w, "No command specified")
			return
		}
		policy := RESTART_POLICY_ALWAYS
		for k, v := range r.Form {
			if strings.ToLower(k) != "restartpolicy" {
				continue
			}
			for _, val := range v {
				switch strings.ToLower(val) {
				case "always":
					policy = RESTART_POLICY_ALWAYS
				case "never":
					policy = RESTART_POLICY_NEVER
				case "onfailure":
					policy = RESTART_POLICY_ONFAILURE
				}
			}
		}
		proc, err := startUnitHelper(s.installRootdir, unit, command,
			s.makeAppEnv(unit), policy)

		if err != nil {
			serverError(w, err)
			return
		}
		fmt.Fprintf(w, "%d", proc.Pid)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) getStatus(unitname string) (string, error) {
	if !IsUnitExist(s.installRootdir, unitname) {
		return UnitStatusUnknown, fmt.Errorf("Unit %s does not exist", unitname)
	}
	u, err := OpenUnit(s.installRootdir, unitname)
	if err != nil {
		glog.Errorf("Error opening unit %s: %v\n", unitname, err)
		return UnitStatusUnknown, err
	}
	defer u.Close()
	st, err := u.GetStatus()
	if err != nil {
		glog.Errorf("Error getting status of unit %s: %v\n", unitname, err)
		return UnitStatusUnknown, err
	}
	return string(st), nil
}

func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		unit := ""
		if len(parts) > 3 {
			unit = strings.Join(parts[3:], "/")
		}
		status, err := s.getStatus(unit)
		if err != nil {
			serverError(w, fmt.Errorf("getStatus(): %v", err))
			return
		}
		fmt.Fprintf(w, "%s", status)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) envHandler(w http.ResponseWriter, r *http.Request) {
	// POST
	// curl -X POST -d "val=bar" http://localhost:8000/rest/v1/unit/env/<unitname>/varname
	unit, err := getURLPart(4, r.URL.Path)

	if err != nil {
		badRequest(w, "Incorrect url format, no unit name")
		return
	}
	name, err := getURLPart(5, r.URL.Path)
	if err != nil {
		badRequest(w, "Incorrect url format, no variable name")
		return
	}
	switch r.Method {
	case "GET":
		value, found := s.env.Get(unit, name)
		if !found {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, value)
	case "POST":
		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, "envHandler ParseForm() err: %v", err)
			return
		}
		value := r.FormValue("val")
		s.env.Add(unit, name, value)
		fmt.Fprintf(w, "OK")
	case "DELETE":
		s.env.Delete(unit, name)
		fmt.Fprintf(w, "OK")
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

func (s *Server) uptimeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		fmt.Fprintf(w, "55") // random, donesn't matter, could just
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) logsHandler(w http.ResponseWriter, r *http.Request) {
	// additional params: need PID of process
	switch r.Method {
	case "GET":
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		unit := ""
		if len(parts) > 3 {
			unit = strings.Join(parts[3:], "/")
		}
		n := 0
		numBytes := 0
		lines := r.FormValue("lines")
		strBytes := r.FormValue("bytes")
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
		if len(logbuf) > 1 && unit == "" {
			badRequest(w, "A unit name is required when getting logs from a pod with multiple units")
			return
		}
		logs := getLogBuffer(unit, n)
		var buffer bytes.Buffer
		for _, entry := range logs {
			buffer.WriteString(entry.Line)
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
		s, err := tailFile(path, lines, int64(numBytes))
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

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
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

func getUnitFromPath(path string) string {
	// The path is always /rest/v1/<endpoint>/<unit> for unit-specific
	// endpoints.
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	unit := ""
	if len(parts) > 3 {
		unit = strings.Join(parts[3:], "/")
	}
	return unit
}

func (s *Server) createMountHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		if r.Body == nil {
			msg := "missing body in mount request"
			glog.Errorf("%s", msg)
			badRequest(w, msg)
			return
		}
		mount, err := ioutil.ReadAll(r.Body)
		if err != nil {
			glog.Errorf("reading body in createMountHandler(): %v", err)
			serverError(w, err)
			return
		}
		err = createMount(s.installRootdir, string(mount))
		if err != nil {
			glog.Errorf("Error creating mount %s: %v", string(mount), err)
			if perr, ok := err.(*ParameterError); ok {
				// Return the original error.
				badRequest(w, perr.err.Error())
				return
			}
			serverError(w, err)
		}
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) attachMountHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		unit := getUnitFromPath(r.URL.Path)
		if unit == "" {
			msg := "Error, missing unit name in mount attach request"
			glog.Errorf("%s", msg)
			badRequest(w, msg)
			return
		}
		if err := r.ParseForm(); err != nil {
			msg := fmt.Sprintf("Error parsing form for mount attach: %v", err)
			glog.Errorf("%s", msg)
			badRequest(w, msg)
			return
		}
		mountname := r.FormValue("name")
		mountpath := r.FormValue("path")
		if mountname == "" || mountpath == "" {
			msg := fmt.Sprintf("Missing parameters for mount attach, got: %+v",
				r.Form)
			badRequest(w, msg)
			return
		}
		err := attachMount(s.installRootdir, unit, mountname, mountpath)
		if err != nil {
			glog.Errorf("Error attaching mount %s: %v", mountname, err)
			serverError(w, err)
			return
		}
		return
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) deployHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		unit := getUnitFromPath(r.URL.Path)

		if err := r.ParseForm(); err != nil {
			glog.Errorf("Parsing form failed: %v", err)
			fmt.Fprintf(w, "appHandler ParseForm() err: %v", err)
			return
		}

		image := r.FormValue("image")
		if image == "" {
			msg := fmt.Sprintf("No image specified; Form: %v PostForm: %v",
				r.Form, r.PostForm)
			glog.Error(msg)
			badRequest(w, msg)
			return
		}
		// if we don't have a username and password, these values will
		// be empty and they won't be used by pullAndExtractImage
		username := r.FormValue("username")
		password := r.FormValue("password")
		server := r.FormValue("server")
		// Tosi requires that we start with http:// or https://
		// always try to use https://
		if server != "" && !strings.HasPrefix(server, "http") {
			server = "https://" + server
		}

		glog.Infof("Creating new unit '%s' in %s\n", unit, s.installRootdir)
		u, err := OpenUnit(s.installRootdir, unit)
		if err != nil {
			glog.Errorf("opening unit %s for package deploy: %v", unit, err)
			serverError(w, err)
			return
		}
		defer u.Close()
		rootfs := u.GetRootfs()

		err = pullAndExtractImage(image, rootfs, server, username, password)
		if err != nil {
			glog.Errorf("pulling image %s: %v", image, err)
			serverError(w, err)
			return
		}

	default:
		http.NotFound(w, r)
	}
}

func (s *Server) getHandlers() {
	s.mux = http.ServeMux{}
	s.mux.HandleFunc("/rest/v1/logs/", s.logsHandler)
	s.mux.HandleFunc("/rest/v1/file/", s.fileHandler)
	s.mux.HandleFunc("/rest/v1/deploy/", s.deployHandler)
	s.mux.HandleFunc("/rest/v1/start/", s.startHandler)
	s.mux.HandleFunc("/rest/v1/status/", s.statusHandler)
	s.mux.HandleFunc("/rest/v1/env/", s.envHandler)
	s.mux.HandleFunc("/rest/v1/mount/", s.attachMountHandler)
	s.mux.HandleFunc("/rest/v1/mount", s.createMountHandler)
	s.mux.HandleFunc("/rest/v1/resizevolume", s.resizevolumeHandler)
	s.mux.HandleFunc("/rest/v1/ping", s.pingHandler)
	s.mux.HandleFunc("/rest/v1/version", s.versionHandler)
}

func (s *Server) ListenAndServe(addr string, insecure bool) {
	s.getHandlers()

	if insecure {
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
