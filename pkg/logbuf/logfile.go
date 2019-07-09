package logbuf

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sync"

	"github.com/golang/glog"
)

// This can be written by multiple goroutines (we launch a separate
// goroutine for each stream) so we'll protect writing with a
// mutex. According to stack overflow File pointer operations are
// atomic in linux after kernel 3.14 but we keep around other state
// that we'll want to protect.
type RotatingFile struct {
	sync.Mutex
	directory   string
	filename    string
	maxSize     int
	currentSize int
	fp          *os.File
}

func NewRotatingFile(directory, filename string, maxSize int) (*RotatingFile, error) {
	err := os.MkdirAll(directory, 0640)
	if err != nil {
		return nil, fmt.Errorf("Error creating log directory %s: %v", directory, err)
	}
	filePath := path.Join(directory, filename)
	fp, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0640)
	if err != nil {
		return nil, fmt.Errorf("Error creating logfile %s: %v", filePath, err)
	}
	rf := &RotatingFile{
		directory:   directory,
		filename:    filename,
		maxSize:     maxSize,
		currentSize: 0,
		fp:          fp,
	}
	return rf, nil
}

func (rf *RotatingFile) Write(b []byte) (int, error) {
	rf.Lock()
	defer rf.Unlock()
	if rf.fp == nil {
		return 0, nil
	}
	n, err := rf.fp.Write(b)
	rf.currentSize += n
	if err != nil {
		return n, err
	}

	if rf.currentSize > rf.maxSize {
		err := rf.rotate()
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (rf *RotatingFile) filePath() string {
	return path.Join(rf.directory, rf.filename)
}

func (rf *RotatingFile) rotatedFilePath() string {
	return rf.filePath() + ".1"
}

func (rf *RotatingFile) rotate() error {
	filepath := rf.filePath()
	rotatedFilePath := rf.rotatedFilePath()
	err := os.Rename(filepath, rotatedFilePath)
	if err != nil {
		glog.Errorf("Error rotating logfile: could not rename logfile to %s: %v", rotatedFilePath, err)
	}
	newFP, errCreate := os.OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0640)
	rf.currentSize = 0
	if err != nil {
		rf.fp = nil
		return fmt.Errorf("Error rotating logfile: could not create logfile %s after rotation: %v", filepath, errCreate)
	}
	rf.fp = newFP
	return nil
}

type JsonLogWriter struct {
	sink io.Writer
}

func NewJsonLogWriter(sink io.Writer) *JsonLogWriter {
	logger := &JsonLogWriter{
		sink: sink,
	}
	return logger
}

func (o JsonLogWriter) Write(entry LogEntry) error {
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = o.sink.Write(b)
	if err != nil {
		return err
	}
	_, err = o.sink.Write([]byte("\n"))
	return err
}
