// aws s3 cp itzo s3://itzo-download/ --acl public-read

package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/shlex"
)

const (
	MULTIPART_FILE_NAME = "file"
	MULTIPART_PKG_NAME  = "pkg"
	DEFAULT_ROOTDIR     = "/tmp/milpa/units"
	ITZO_VERSION        = "1.0"
)

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
	e := os.Environ()
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

func isExecutable(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	header := make([]byte, 4)
	n1, err := file.Read(header)
	if err != nil {
		return false
	}
	if n1 < 4 {
		return false
	}
	if (header[0] == 0x7F && string(header[1:]) == "ELF") ||
		string(header[0:2]) == "#!" {
		return true
	}
	return false
}

func ensureExecutable(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("Could not stat file at %s", path)
	}
	perms := fi.Mode()
	if (perms & 0110) == 0 {
		os.Chmod(path, perms|0110)
	}
	return nil
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
		commandParts, err := shlex.Split(command)
		if err != nil {
			serverError(w, err)
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
		appid, err := startUnitHelper(s.installRootdir, unit, commandParts,
			s.makeAppEnv(unit), policy)
		if err != nil {
			serverError(w, err)
			return
		}
		fmt.Fprintf(w, "%d", appid)
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

func (s *Server) fileUploadHandler(w http.ResponseWriter, r *http.Request) {
	// example usage: curl -X POST http://localhost:8000/file/dest.txt -Ffile=@testfile.txt
	switch r.Method {
	case "POST":
		key, err := getURLPart(2, r.URL.RawPath)
		if err != nil {
			badRequest(w, "Incorrect url format")
			return
		}

		formFile, _, err := r.FormFile(MULTIPART_FILE_NAME)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		localPath, err := url.PathUnescape(key)
		if err != nil {
			msg := fmt.Sprintf("Error unescaping path: %s", err.Error())
			badRequest(w, msg)
			return
		}
		dirpath := filepath.Dir(localPath)
		err = os.MkdirAll(dirpath, 0760)
		if err != nil {
			serverError(w, err)
			return
		}
		destFile, err := os.Create(localPath)
		if err != nil {
			serverError(w, err)
			return
		}
		_, err = io.Copy(destFile, formFile)
		if err != nil {
			serverError(w, err)
			return
		}
		_ = destFile.Close()
		// if the user uploaded an elf, make it executable
		if isExecutable(localPath) {
			if err := ensureExecutable(localPath); err != nil {
				serverError(w, err)
				return
			}
		}
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
		lines := r.FormValue("lines")
		if lines != "" {
			if i, err := strconv.Atoi(lines); err == nil {
				n = i
			}
		}
		logs := getLogBuffer(unit, n)
		var buffer bytes.Buffer
		for _, entry := range logs {
			buffer.WriteString(entry.Line)
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "%s", buffer.String())
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/file") {
		s.fileUploadHandler(w, r)
	} else {
		s.mux.ServeHTTP(w, r)
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

func downloadFile(url string) (filename string, n int64, err error) {
	resp, err := http.Get(url)
	if err != nil {
		err = fmt.Errorf("Error downloading file: %s", err)
		return
	}
	if resp.StatusCode != 200 {
		err = fmt.Errorf("Download error, server responded with status code %d", resp.StatusCode)
		return
	}
	reader := bufio.NewReader(resp.Body)
	defer resp.Body.Close()
	return saveFile(reader)
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

func getServerFromImage(image string) (string, string) {
	u, err := url.Parse(image)
	if err != nil {
		glog.Warningf(
			"Trouble parsing image string %s, trying to continue", image)
		return "", image
	}
	return u.Scheme + u.Host, u.Path
}

func (s *Server) deployHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		unit := ""
		if len(parts) > 3 {
			unit = strings.Join(parts[3:], "/")
		}

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
		url, image := getServerFromImage(image)
		// if we don't have a username and password, these values will
		// be empty and they won't be used by pullAndExtractImage
		username := r.FormValue("username")
		password := r.FormValue("password")
		u, err := OpenUnit(s.installRootdir, unit)
		if err != nil {
			glog.Errorf("opening unit %s for package deploy: %v", unit, err)
			serverError(w, err)
			return
		}
		defer u.Close()
		rootfs := u.GetRootfs()

		err = pullAndExtractImage(image, rootfs, url, username, password)
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
	s.mux.HandleFunc("/rest/v1/deploy/", s.deployHandler)
	s.mux.HandleFunc("/rest/v1/start/", s.startHandler)
	s.mux.HandleFunc("/rest/v1/status/", s.statusHandler)

	s.mux.HandleFunc("/rest/v1/env/", s.envHandler)
	s.mux.HandleFunc("/rest/v1/resizevolume", s.resizevolumeHandler)
	s.mux.HandleFunc("/rest/v1/ping", s.pingHandler)
	s.mux.HandleFunc("/rest/v1/version", s.versionHandler)
}

func (s *Server) ListenAndServe(addr string) {
	s.getHandlers()
	s.httpServer = &http.Server{Addr: addr, Handler: s}
	glog.Fatalln(s.httpServer.ListenAndServe())
}
