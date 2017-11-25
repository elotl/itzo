package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/google/shlex"
	"github.com/gorilla/mux"
	"golang.org/x/sys/unix"
)

const MULTIPART_FILE_NAME = "file"

type Server struct {
	env        StringMap
	httpServer *http.Server
}

func New() *Server {
	return &Server{
		env: StringMap{data: map[string]string{}},
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

func (s *Server) appHandler(w http.ResponseWriter, r *http.Request) {
	// PUT
	// Path: milpa/app/
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
	// Path: env/{varname}
	// parameters: val

	// curl -X POST -d "val=bar" http://localhost:8000/env/foo
	switch r.Method {
	case "GET":
		vars := mux.Vars(r)
		key := vars["name"]
		value, found := s.env.Get(key)
		if !found {
			http.NotFound(w, r)
		}
		fmt.Fprintf(w, value)
	case "POST":
		vars := mux.Vars(r)
		if err := r.ParseForm(); err != nil {
			fmt.Fprintf(w, "envHandler ParseForm() err: %v", err)
			return
		}
		key := vars["name"]
		value := r.FormValue("val")
		s.env.Add(key, value)
		fmt.Fprintf(w, "OK")
	case "DELETE":
		vars := mux.Vars(r)
		key := vars["name"]
		s.env.Delete(key)
		fmt.Fprintf(w, "OK")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	// GET
	// milpa/health
	switch r.Method {
	case "GET":
		fmt.Fprintf(w, "OK")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) fileUploadHandler(w http.ResponseWriter, r *http.Request) {
	// POST
	// file/server_path
	// example usage: curl -X POST http://localhost:8000/file/dest.txt -Ffile=@testfile.txt
	switch r.Method {
	case "POST":
		formFile, _, err := r.FormFile(MULTIPART_FILE_NAME)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		localPath := r.URL.Path
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
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) logsHandler(w http.ResponseWriter, r *http.Request) {
	// POST
	// milpa/log
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
			const c = unix.LINUX_REBOOT_CMD_HALT
			_ = unix.Reboot(-(c>>31)<<31 | c&(1<<31-1))
		}()
	default:
		http.NotFound(w, r)
	}
}

// We create the router and pass it back cause we end up using this in
// unit tests.
func (s *Server) getHandlers() *mux.Router {
	r := mux.NewRouter()
	// Gorilla.mux seems to have some terrible handling of paths and does
	// cleaning of paths stragely (to me, at least). It shouldn't have been
	// used because getting the embedded slashes to work in the /file handler
	// took 10x more time than the framework saved.
	r.SkipClean(true)
	r.HandleFunc("/env/{name}", s.envHandler).Methods("GET", "POST", "DELETE")
	r.PathPrefix("/file/").Handler(http.StripPrefix("/file/", http.HandlerFunc(s.fileUploadHandler))).Methods("POST")
	r.HandleFunc("/app", s.appHandler).Methods("PUT")
	r.HandleFunc("/milpa/health", s.healthcheckHandler).Methods("GET")
	r.HandleFunc("/os/reboot", s.rebootHandler).Methods("POST")
	r.HandleFunc("/milpa/logs/", s.logsHandler).Methods("POST")
	return r
}

func (s *Server) ListenAndServe(addr string) {
	http.Handle("/", s.getHandlers())
	s.httpServer = &http.Server{Addr: addr, Handler: nil}
	log.Fatal(s.httpServer.ListenAndServe())
}
