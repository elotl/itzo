// aws s3 cp itzo s3://itzo-download/ --acl public-read

package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/shlex"
	"golang.org/x/sys/unix"
)

const MULTIPART_FILE_NAME = "file"

type Server struct {
	env        StringMap
	httpServer *http.Server
	mux        http.ServeMux
	startTime  time.Time
}

func New() *Server {
	return &Server{
		env:       StringMap{data: map[string]string{}},
		startTime: time.Now().UTC(),
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

func isElf(path string) bool {
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
	if header[0] == 0x7F && string(header[1:]) == "ELF" {
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
		cmd := exec.Command(commandParts[0], commandParts[1:]...)
		cmd.Env = s.makeAppEnv()
		err = cmd.Start()
		if err != nil {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			serverError(w, err)
			return
		}
		fmt.Fprintf(w, "%d", cmd.Process.Pid)
		// todo: capture stdout logs
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
		//fmt.Println("url:", r.URL.RawPath)
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
		if isElf(localPath) {
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
	case "POST":
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
}

func (s *Server) ListenAndServe(addr string) {
	s.getHandlers()
	s.httpServer = &http.Server{Addr: addr, Handler: s}
	log.Fatal(s.httpServer.ListenAndServe())
}
