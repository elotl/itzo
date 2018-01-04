// aws s3 cp itzo s3://itzo-download/ --acl public-read

package server

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/google/shlex"
	"golang.org/x/sys/unix"
)

const (
	MULTIPART_FILE_NAME = "file"
	MULTIPART_PKG_NAME  = "pkg"
	DEFAULT_ROOTDIR     = "/tmp/milpa/units"
)

type Server struct {
	env        StringMap
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
		env:            StringMap{data: map[string]string{}},
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

func (s *Server) makeAppEnv() []string {
	e := os.Environ()
	for _, d := range s.env.Items() {
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

func (s *Server) appHandler(w http.ResponseWriter, r *http.Request) {
	// query parameters
	// command
	switch r.Method {
	case "PUT":
		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, "appHandler ParseForm() err: %v", err)
			return
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
		appid, err := startUnitHelper(s.installRootdir, "", commandParts, s.makeAppEnv())
		if err != nil {
			serverError(w, err)
			return
		}
		fmt.Fprintf(w, "%d", appid)
		// todo: capture stdout logs
	default:
		http.NotFound(w, r)
	}
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
		if len(parts) > 2 {
			unit = strings.Join(parts[2:], "/")
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
		appid, err := startUnitHelper(s.installRootdir, unit, commandParts, s.makeAppEnv())
		if err != nil {
			serverError(w, err)
			return
		}
		fmt.Fprintf(w, "%d", appid)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) envHandler(w http.ResponseWriter, r *http.Request) {
	// POST
	// curl -X POST -d "val=bar" http://localhost:8000/env/foo
	key, err := getURLPart(2, r.URL.Path)
	if err != nil {
		badRequest(w, "Incorrect url format")
		return
	}
	switch r.Method {
	case "GET":
		// vars := mux.Vars(r)
		// key := vars["name"]
		value, found := s.env.Get(key)
		if !found {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, value)
	case "POST":
		//vars := mux.Vars(r)
		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, "envHandler ParseForm() err: %v", err)
			return
		}
		//key := vars["name"]
		value := r.FormValue("val")
		s.env.Add(key, value)
		fmt.Fprintf(w, "OK")
	case "DELETE":
		//vars := mux.Vars(r)
		//key := vars["name"]
		s.env.Delete(key)
		fmt.Fprintf(w, "OK")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		fmt.Fprintf(w, "OK")
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
		if len(parts) > 2 {
			unit = strings.Join(parts[2:], "/")
		}
		n := 0
		lines := r.FormValue("lines")
		if lines != "" {
			if i, err := strconv.Atoi(lines); err == nil {
				n = i
			}
		}
		logs := getLogBuffer(unit, n)
		json, err := json.Marshal(logs)
		if err != nil {
			serverError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "%s", json)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) rebootHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		fmt.Fprintf(w, "OK")
		// todo, stop serving
		go func() {
			time.Sleep(500 * time.Millisecond)
			// from https://github.com/golang/go/issues/9584
			const c = unix.LINUX_REBOOT_CMD_RESTART
			syscall.Sync()
			_ = unix.Reboot(-(c>>31)<<31 | c&(1<<31-1))
		}()
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

type Link struct {
	dst      string
	src      string
	linktype byte
	mode     os.FileMode
	uid      int
	gid      int
}

func extractAndInstall(rootdir, unit, filename string) (err error) {
	rootfs := getUnitRootfs(rootdir, unit)
	err = os.MkdirAll(rootfs, 0700)
	if err != nil {
		glog.Errorln("creating rootfs", rootfs, ":", err)
		return err
	}

	f, err := os.Open(filename)
	if err != nil {
		glog.Errorln("opening package file:", err)
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		glog.Errorln("uncompressing package:", err)
		return err
	}
	defer gzr.Close()

	var links []Link

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			glog.Errorln("extracting package:", err)
			return err
		}

		name := header.Name
		if name == "ROOTFS" {
			continue
		}
		if len(name) < 7 || name[:7] != "ROOTFS/" {
			glog.Warningln("file outside of ROOTFS in package:", name)
			continue
		}
		name = filepath.Join(rootfs, name[7:])

		switch header.Typeflag {
		case tar.TypeDir: // directory
			glog.Infoln("d", name)
			os.Mkdir(name, os.FileMode(header.Mode))
		case tar.TypeReg: // regular file
			glog.Infoln("f", name)
			data := make([]byte, header.Size)
			read_so_far := int64(0)
			for read_so_far < header.Size {
				n, err := tr.Read(data[read_so_far:])
				if err != nil && err != io.EOF {
					glog.Errorln("extracting", name, ":", err)
					return err
				}
				read_so_far += int64(n)
			}
			if read_so_far != header.Size {
				glog.Errorf("f %s error: read %d bytes, but size is %d bytes", name, read_so_far, header.Size)
			}
			ioutil.WriteFile(name, data, os.FileMode(header.Mode))
		case tar.TypeLink, tar.TypeSymlink:
			linkname := header.Linkname
			if len(linkname) >= 7 && linkname[:7] == "ROOTFS/" {
				linkname = filepath.Join(rootfs, linkname[7:])
			}
			// Links might point to files or directories that have not been
			// extracted from the tarball yet. Create them after going through
			// all entries in the tarball.
			links = append(links, Link{linkname, name, header.Typeflag, os.FileMode(header.Mode), header.Uid, header.Gid})
			continue
		default:
			glog.Warningf("unknown type while untaring: %d", header.Typeflag)
			continue
		}
		err = os.Chown(name, header.Uid, header.Gid)
		if err != nil {
			glog.Warningf("warning: chown %s type %d uid %d gid %d: %v", name, header.Typeflag, header.Uid, header.Gid, err)
		}
	}

	for _, link := range links {
		os.Remove(link.src) // Remove link in case it exists.
		if link.linktype == tar.TypeSymlink {
			glog.Infoln("s", link.src)
			err = os.Symlink(link.dst, link.src)
			if err != nil {
				glog.Errorf("creating symlink %s -> %s: %v", link.src, link.dst, err)
				return err
			}
			err = os.Lchown(link.src, link.uid, link.gid)
			if err != nil {
				glog.Warningf("warning: chown symlink %s uid %d gid %d: %v", link.src, link.uid, link.gid, err)
			}
		}
		if link.linktype == tar.TypeLink {
			glog.Infoln("h", link.src)
			err = os.Link(link.dst, link.src)
			if err != nil {
				glog.Errorf("creating hardlink %s -> %s: %v", link.src, link.dst, err)
				return err
			}
			err = os.Chmod(link.src, link.mode)
			if err != nil {
				glog.Errorf("chmod hardlink %s %d: %v", link.src, link.mode, err)
				return err
			}
			err = os.Chown(link.src, link.uid, link.gid)
			if err != nil {
				glog.Warningf("warning: chown hardlink %s uid %d gid %d: %v", link.src, link.uid, link.gid, err)
			}
		}
	}

	return nil
}

func (s *Server) deployFileHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		unit := ""
		if len(parts) > 2 {
			unit = strings.Join(parts[2:], "/")
		}
		formFile, _, err := r.FormFile(MULTIPART_PKG_NAME)
		if err != nil {
			glog.Errorln("parsing form for package deploy:", err)
			serverError(w, err)
			return
		}
		defer formFile.Close()
		pkgfile, n, err := saveFile(formFile)
		if err != nil {
			glog.Errorln("saving file for package deploy:", err)
			serverError(w, err)
			return
		}
		defer os.Remove(pkgfile)
		glog.Infof("package for unit \"%s\" saved as: %s (%d bytes)",
			unit, pkgfile, n)
		if err = extractAndInstall(s.installRootdir, unit, pkgfile); err != nil {
			glog.Errorln("extracting and installing package:", err)
			serverError(w, err)
			return
		}
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) deployURLHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, "appHandler ParseForm() err: %v", err)
			return
		}
		url := r.FormValue("url")
		if url == "" {
			badRequest(w, "No url specified")
			return
		}
		unit := r.FormValue("unit")
		if unit == "" {
			badRequest(w, "No unit name specified")
			return
		}

		pkgfile, _, err := downloadFile(url)
		if err != nil {
			err := fmt.Errorf("Error downloading package from %s to node: %s", url, err)
			glog.Errorln(err)
			serverError(w, err)
			return
		}

		defer os.Remove(pkgfile)
		if err = extractAndInstall(s.installRootdir, unit, pkgfile); err != nil {
			glog.Errorln("extracting and installing package:", err)
			serverError(w, err)
			return
		}

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

func (s *Server) getHandlers() {
	// The /file/<path> endpoint is a real pain point
	// by default, go's handlers will strip out double slashes //
	// as well as a slash followed by an encoded slash (/%2F)
	// So we create our own handler that handles /file specially
	// and defer all other endpoints to our mux
	s.mux = http.ServeMux{}
	s.mux.HandleFunc("/env/", s.envHandler)
	s.mux.HandleFunc("/app/", s.appHandler) // OSv seems to need trailing slash
	s.mux.HandleFunc("/milpa/health", s.healthcheckHandler)
	s.mux.HandleFunc("/os/uptime", s.uptimeHandler)
	s.mux.HandleFunc("/os/reboot", s.rebootHandler)
	s.mux.HandleFunc("/milpa/logs/", s.logsHandler)
	s.mux.HandleFunc("/milpa/deployfile/", s.deployFileHandler)
	// Remove the next two once milpa always specifies unit for deploy.
	s.mux.HandleFunc("/milpa/deployfile", s.deployFileHandler)
	// remove deploy endpoint when we are all upgraded to use deployfile
	s.mux.HandleFunc("/milpa/deploy/", s.deployFileHandler)
	s.mux.HandleFunc("/milpa/deploy", s.deployFileHandler)

	s.mux.HandleFunc("/milpa/deployurl/", s.deployURLHandler)

	s.mux.HandleFunc("/milpa/start/", s.startHandler)
	s.mux.HandleFunc("/milpa/resizevolume", s.resizevolumeHandler)
}

func (s *Server) ListenAndServe(addr string) {
	s.getHandlers()
	s.httpServer = &http.Server{Addr: addr, Handler: s}
	glog.Fatalln(s.httpServer.ListenAndServe())
}
