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
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/golang/glog"
)

const (
	PIPE_UNIT_STDOUT = "unit-stdout"
	PIPE_UNIT_STDERR = "unit-stderr"
)

var UNIT_PIPES = []string{PIPE_UNIT_STDOUT, PIPE_UNIT_STDERR}

type LogPipe struct {
	Unitdir string
	Pipes   map[string]*os.File
}

func checkName(name string) {
	for _, pipe := range UNIT_PIPES {
		if name == pipe {
			return
		}
	}
	panic(fmt.Sprintf("Invalid pipe name %s", name))
}

func (l *LogPipe) Chown(uid, gid int) error {
	for _, p := range l.Pipes {
		err := p.Chown(int(uid), int(gid))
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *LogPipe) OpenWriter(name string) (fp *os.File, err error) {
	// Open pipe with name "name" for writing. The reader side was created and
	// connected in New().
	checkName(name)
	pipepath := filepath.Join(l.Unitdir, name)
	fp, err = os.OpenFile(pipepath, os.O_WRONLY, 0600)
	if err != nil {
		glog.Errorf("Error opening %s: %v", pipepath, err)
		return nil, err
	}
	l.Pipes[name] = fp
	return fp, nil
}

func (l *LogPipe) readFromPipe(name string, callback func(string)) {
	pipepath := filepath.Join(l.Unitdir, name)
	pf, err := os.OpenFile(pipepath, os.O_RDONLY, 0600)
	if err != nil {
		glog.Errorf("Error opening %s: %v", pipepath, err)
		return
	}
	defer pf.Close()
	r := bufio.NewReader(pf)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			// Probably the helper exited, thus we got an EOF.
			if err != io.EOF {
				glog.Errorf("Error reading from pipe %v: %v", pipepath, err)
			}
			break
		}
		callback(line)
	}
}

func (l *LogPipe) Remove() {
	// Best effort to clean up log pipes. Closing them will make sure that any
	// running goroutines reading from them will get an EOF.
	glog.Infof("Closing and removing all log pipes in unit dir %s", l.Unitdir)
	for _, name := range UNIT_PIPES {
		pipepath := filepath.Join(l.Unitdir, name)
		p := l.Pipes[name]
		l.Pipes[name] = nil
		if p != nil {
			p.Close()
			os.Remove(pipepath)
			continue
		}
		fp, err := os.OpenFile(pipepath, os.O_WRONLY|syscall.O_NONBLOCK, 0600)
		if err != nil {
			if os.IsNotExist(err) {
				glog.Infof("%s does not exist", pipepath)
				continue
			} else {
				// Try to remove it anyway.
				glog.Infof("Error opening %s: %v", pipepath, err)
			}
		} else {
			fp.Close()
		}
		os.Remove(pipepath)
	}
}

func NewLogPipe(dir string) (*LogPipe, error) {
	// Create named pipes that the unit will use for stdout/stderr. A separate
	// one is created for the helper process itself, so outputs from the helper
	// and the application are not intertwined.
	l := LogPipe{
		Unitdir: dir,
		Pipes:   make(map[string]*os.File),
	}
	for _, name := range UNIT_PIPES {
		pipepath := filepath.Join(l.Unitdir, name)
		err := syscall.Mkfifo(pipepath, 0600)
		if err != nil && !os.IsExist(err) {
			glog.Errorf("Error creating log pipe %s: %v", pipepath, err)
			l.Remove()
			return nil, err
		}
	}
	return &l, nil
}

func (l *LogPipe) StartReader(name string, cb func(string)) {
	checkName(name)
	go l.readFromPipe(name, cb)
}

func (l *LogPipe) StartAllReaders(cb func(string)) {
	for _, name := range UNIT_PIPES {
		l.StartReader(name, cb)
	}
}
