// aws s3 cp itzo s3://itzo-download/ --acl public-read

package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/mount"
	"github.com/golang/glog"
	"github.com/gorilla/websocket"
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

type WebsocketParams struct {
	// Time allowed to write data to the client.
	writeWait time.Duration
	// Time allowed to read the next pong message from the client.
	pongWait time.Duration
	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod time.Duration
	// Poll for changes with this period.
	filePeriod time.Duration
}

type Server struct {
	env           EnvStore
	httpServer    *http.Server
	mux           http.ServeMux
	startTime     time.Time
	podController *PodController
	unitMgr       *UnitManager
	wsParams      WebsocketParams
	wsUpgrader    websocket.Upgrader
	// Packages will be installed under this directory (created if it does not
	// exist).
	installRootdir string
}

func New(rootdir string) *Server {
	if rootdir == "" {
		rootdir = DEFAULT_ROOTDIR
	}
	mounter := mount.NewOSMounter(rootdir)
	um := NewUnitManager(rootdir)
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
		wsParams: WebsocketParams{
			writeWait:  10 * time.Second,
			pingPeriod: (2 * time.Second * 9) / 10,
			filePeriod: 100 * time.Millisecond,
		},
	}
}

func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		status, err := s.podController.GetStatus()
		if err != nil {
			serverError(w, err)
			return
		}
		reply := api.PodStatusReply{
			UnitStatuses: status,
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
		err = s.podController.UpdatePod(&params)
		if err != nil {
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
	// additional params: need PID of process
	switch r.Method {
	case "GET":
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		unit := ""
		if len(parts) > 3 {
			unit = strings.Join(parts[3:], "/")
		}

		// todo, this is a bit messy here, break it out if possible
		follow := r.FormValue("follow")
		if follow != "" {
			// Bug: if the unit gets closed, we don't know about the closure
			unitName, _ := s.unitMgr.CleanUnitName(unit)
			logBuffer, err := s.unitMgr.GetLogBuffer(unit)
			if err != nil {
				badRequest(w, err.Error())
			}
			ws, err := s.wsUpgrader.Upgrade(w, r, nil)
			if err != nil {
				serverError(w, err)
				return
			}
			s.RunLogTailer(ws, unitName, logBuffer)
			return
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

		logs, err := s.unitMgr.ReadLogBuffer(unit, n)
		if err != nil {
			badRequest(w, err.Error())
			return

		}
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

func (s *Server) RunLogTailer(ws *websocket.Conn, unitName string, logBuffer *logbuf.LogBuffer) {
	pingTicker := time.NewTicker(s.wsParams.pingPeriod)
	fileTicker := time.NewTicker(s.wsParams.filePeriod)
	closed := make(chan struct{})

	defer func() {
		pingTicker.Stop()
		fileTicker.Stop()
		ws.Close()
	}()

	// Reader goroutine just listens for pings and the client disconnecting
	go func() {
		ws.SetReadLimit(512)
		ws.SetReadDeadline(time.Now().Add(s.wsParams.pongWait))
		ws.SetPongHandler(func(string) error {
			ws.SetReadDeadline(time.Now().Add(s.wsParams.pongWait))
			return nil
		})
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					glog.Info("closing log tailing connection")
				} else {
					glog.Errorln("Closing connection after error:", err)
				}
				close(closed)
				return
			}
		}
	}()

	lastOffset := logBuffer.GetOffset()
	for {
		select {
		case <-closed:
			return
		case <-fileTicker.C:
			unitRunning := s.unitMgr.UnitRunning(unitName)
			if !unitRunning {
				_ = ws.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}

			var entries []logbuf.LogEntry
			entries, lastOffset = logBuffer.ReadSince(lastOffset)
			if len(entries) > 0 {
				msg := make([]byte, 0, 1024)
				for i := 0; i < len(entries); i++ {
					msg = append(msg, []byte(entries[i].Line)...)
				}
				_ = ws.SetWriteDeadline(time.Now().Add(s.wsParams.writeWait))
				err := ws.WriteMessage(websocket.TextMessage, msg)
				if err != nil {
					return
				}
			}
		case <-pingTicker.C:
			_ = ws.SetWriteDeadline(time.Now().Add(s.wsParams.writeWait))
			err := ws.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				return
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
	s.mux = http.ServeMux{}
	s.mux.HandleFunc("/rest/v1/logs/", s.logsHandler)
	s.mux.HandleFunc("/rest/v1/file/", s.fileHandler)
	// The updatepod endpoint is used to send in a full PodParameters struct.
	s.mux.HandleFunc("/rest/v1/updatepod", s.updateHandler)
	// This endpoint gives back the status of the whole pod.
	s.mux.HandleFunc("/rest/v1/status", s.statusHandler)
	s.mux.HandleFunc("/rest/v1/resizevolume", s.resizevolumeHandler)
	s.mux.HandleFunc("/rest/v1/ping", s.pingHandler)
	s.mux.HandleFunc("/rest/v1/version", s.versionHandler)
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
