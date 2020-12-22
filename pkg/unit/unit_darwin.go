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

package unit

import (
	"github.com/elotl/itzo/pkg/logbuf"
	"github.com/elotl/itzo/pkg/util/conmap"
	"io"
	"path/filepath"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/elotl/itzo/pkg/mount"
	"github.com/elotl/itzo/pkg/util"
)

const (
	MAX_BACKOFF_TIME                     = 5 * time.Minute
	BACKOFF_RESET_TIME                   = 10 * time.Minute
	CHILD_OOM_SCORE                      = 15 // chosen arbitrarily... kernel will adjust this value
	MaxContainerTerminationMessageLength = 1024 * 4
)

var (
	// List of capabilities granted to units by default. We use the same set as
	// Docker and rkt. See
	// https://docs.docker.com/engine/reference/run/#runtime-privilege-and-linux-capabilities
	// and
	// https://github.com/appc/spec/blob/master/spec/ace.md#oslinuxcapabilities-remove-set
	// for more information.
	defaultCapabilities = []string{
		"CAP_AUDIT_WRITE",
		"CAP_CHOWN",
		"CAP_DAC_OVERRIDE",
		"CAP_FOWNER",
		"CAP_FSETID",
		"CAP_KILL",
		"CAP_MKNOD",
		"CAP_NET_BIND_SERVICE",
		"CAP_NET_RAW",
		"CAP_SETFCAP",
		"CAP_SETGID",
		"CAP_SETPCAP",
		"CAP_SETUID",
		"CAP_SYS_CHROOT",
	}
	sleep = time.Sleep // Allow time.Sleep() to be mocked out in tests.
)

// This is part of the config of docker images.
type HealthConfig struct {
	Test        []string      `json:",omitempty"`
	Interval    time.Duration `json:",omitempty"`
	Timeout     time.Duration `json:",omitempty"`
	StartPeriod time.Duration `json:",omitempty"`
	Retries     int           `json:",omitempty"`
}

// This is the main config struct for docker images.
type Config struct {
	Hostname        string
	Domainname      string
	User            string
	AttachStdin     bool
	AttachStdout    bool
	AttachStderr    bool
	ExposedPorts    map[string]struct{} `json:",omitempty"`
	Tty             bool
	OpenStdin       bool
	StdinOnce       bool
	Env             []string
	Cmd             []string
	Healthcheck     *HealthConfig `json:",omitempty"`
	ArgsEscaped     bool          `json:",omitempty"`
	Image           string
	Volumes         map[string]struct{}
	WorkingDir      string
	Entrypoint      []string
	NetworkDisabled bool   `json:",omitempty"`
	MacAddress      string `json:",omitempty"`
	OnBuild         []string
	Labels          map[string]string
	StopSignal      string   `json:",omitempty"`
	StopTimeout     *int     `json:",omitempty"`
	Shell           []string `json:",omitempty"`
}

type UnitConfig struct {
	api.PodSecurityContext   `json:"podSecurityContext"`
	api.SecurityContext      `json:"securityContext"`
	StartupProbe             *api.Probe `json:",omitempty"`
	ReadinessProbe           *api.Probe `json:",omitempty"`
	LivenessProbe            *api.Probe `json:",omitempty"`
	TerminationMessagePolicy api.TerminationMessagePolicy
	TerminationMessagePath   string
	PodIP                    string
	UseOverlayfs             bool
}

type Unit struct {
	*LogPipe
	Directory   string
	Name        string
	Image       string
	statusPath  string
	config      *Config
	unitConfig  UnitConfig
	stdinPath   string
	stdinCloser chan struct{}
}

func IsUnitExist(rootdir, name string) bool {
	return true
}

func OpenUnit(rootdir, name string) (*Unit, error) {
	u := Unit{}
	return &u, nil
}

func (u *Unit) SaveUnitConfig(unitConfig UnitConfig) error {
	return nil
}

func (u *Unit) SetUnitConfigOverlayfs(useOverlayfs bool) {
	return
}

func (u *Unit) CreateCommand(command []string, args []string) []string {
	return []string{}
}

func (u *Unit) GetEnv() []string {
	return []string{}
}

func (u *Unit) GetWorkingDir() string {
	return ""
}

func (u *Unit) SetImage(image string) error {
	return nil
}

func (u *Unit) Destroy() error {
	return nil
}

func (u *Unit) GetRootfs() string {
	return filepath.Join(u.Directory, "ROOTFS")
}

func (u *Unit) PullAndExtractImage(image, server, username, password string) error {
	return nil
}

func (u *Unit) GetUser(lookup util.UserLookup) (uid, gid uint32, groups []uint32, homedir string, err error) {
	return 0, 0, nil, "", nil
}

func (u *Unit) SetStatus(status *api.UnitStatus) error {
	return nil
}

func (u *Unit) UpdateStatusAttr(ready, started *bool) error {
	return nil
}

func (u *Unit) GetStatus() (*api.UnitStatus, error) {
	return nil, nil
}

func (u *Unit) SetState(state api.UnitState, restarts *int) error {
	return nil
}

func (u *Unit) RunUnitLoop(command, caplist []string, uid, gid uint32, groups []uint32, unitin io.Reader, unitout, uniterr io.Writer, policy api.RestartPolicy) (err error) {
	return nil
}

func (u *Unit) Run(podname, hostname string, command []string, workingdir string, policy api.RestartPolicy, mounter mount.Mounter) error {
	return nil
}

func (u *Unit) OpenStdinWriter() (io.WriteCloser, error) {
	return nil, nil
}

type UnitManager struct {
	rootDir      string
	RunningUnits *conmap.StringOsProcess
	LogBuf       *conmap.StringLogbufLogBuffer
}

func (u UnitManager) StartUnit(s string, s2 string, s3 string, s4 string, s5 string, strings []string, strings2 []string, strings3 []string, policy api.RestartPolicy) error {
	return nil
}

func (u UnitManager) StopUnit(s string) error {
	return nil
}

func (u UnitManager) RemoveUnit(s string) error {
	return nil
}

func (u UnitManager) UnitRunning(s string) bool {
	return true
}

func (u UnitManager) GetLogBuffer(unitName string) (*logbuf.LogBuffer, error) {
	return nil, nil
}

func (u UnitManager) ReadLogBuffer(unitName string, n int) ([]logbuf.LogEntry, error) {
	return nil, nil
}

func (u UnitManager) GetPid(s string) (int, bool) {
	return 0, false
}

func NewUnitManager(rootDir string) *UnitManager {
	return &UnitManager{}
}

func StartUnit(rootdir, podname, hostname, unitname, workingdir, netns string, command []string, policy api.RestartPolicy) error {
	return nil
}