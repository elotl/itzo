package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
)

type Unit struct {
	LogPipe
	Directory  string
	Name       string
	statusFile *os.File
}

type UnitStatus string

const (
	UnitStatusUnknown   = ""
	UnitStatusCreated   = "created"
	UnitStatusRunning   = "running"
	UnitStatusFailed    = "failed"
	UnitStatusSucceeded = "succeeded"
)

func NewUnit(rootdir, name string) (*Unit, error) {
	glog.Infof("Creating new unit '%s' in %s\n", name, rootdir)
	directory := filepath.Join(rootdir, name)
	// Make sure unit directory exists.
	if err := os.MkdirAll(directory, 0700); err != nil {
		glog.Errorf("Error reating unit '%s': %v\n", name, err)
		return nil, err
	}
	spath := filepath.Join(directory, "status")
	f, err := os.OpenFile(spath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		glog.Errorf("Error opening statusfile for unit '%s': %v\n", name, err)
		return nil, err
	}
	u := Unit{
		Directory:  directory,
		Name:       name,
		statusFile: f,
	}
	u.SetStatus(UnitStatusCreated)
	return &u, nil
}

func NewUnitFromDir(unitdir string) (*Unit, error) {
	elements := strings.Split(unitdir, string(filepath.Separator))
	if elements[len(elements)-1] == "" {
		elements = elements[:len(elements)-1]
	}
	if len(elements) <= 1 {
		return nil, fmt.Errorf("Invalid unitdir %s", unitdir)
	}
	rootdir := strings.Join(elements[:len(elements)-1], string(filepath.Separator))
	name := elements[len(elements)-1]
	return NewUnit(rootdir, name)
}

func (u *Unit) Close() {
	if u.statusFile != nil {
		u.statusFile.Close()
		u.statusFile = nil
	}
}

func (u *Unit) GetRootfs() string {
	return filepath.Join(u.Directory, "ROOTFS")
}

func (u *Unit) GetStatus() (UnitStatus, error) {
	_, err := u.statusFile.Seek(0, 0)
	if err != nil {
		glog.Errorf("Error seeking in statusfile for %s\n", u.Name)
		return UnitStatusUnknown, err
	}
	buf := make([]byte, 32)
	n, err := u.statusFile.Read(buf)
	if err != nil {
		glog.Errorf("Error reading statusfile for %s\n", u.Name)
		return UnitStatusUnknown, err
	}
	s := string(buf[:n])
	err = nil
	var status UnitStatus
	switch s {
	case string(UnitStatusCreated):
		status = UnitStatusCreated
	case string(UnitStatusRunning):
		status = UnitStatusRunning
	case string(UnitStatusFailed):
		status = UnitStatusFailed
	case string(UnitStatusSucceeded):
		status = UnitStatusSucceeded
	default:
		status = UnitStatusUnknown
		err := fmt.Errorf("Invalid status for %s: '%v'\n", u.Name, s)
		glog.Error(err)
	}
	return status, err
}

func (u *Unit) SetStatus(status UnitStatus) error {
	glog.Infof("Updating status of unit '%s' to %s\n", u.Name, status)
	_, err := u.statusFile.Seek(0, 0)
	if err != nil {
		glog.Errorf("Error seeking in statusfile for %s\n", u.Name)
		return err
	}
	buf := []byte(status)
	if _, err := u.statusFile.Write(buf); err != nil {
		glog.Errorf("Error updating statusfile for %s\n", u.Name)
		return err
	}
	u.statusFile.Truncate(int64(len(buf)))
	return nil
}
