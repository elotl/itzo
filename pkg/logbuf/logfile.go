package logbuf

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
)

type RotatingFile struct {
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
		return fmt.Errorf("Error rotating logfile: could not rename logfile to %s: %v", rotatedFilePath, err)
	}
	newFP, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("Error rotating logfile: could not create logfile %s after rotation: %v", filepath, err)
	}
	rf.fp = newFP
	return nil
}

type JsonLogWriter struct {
	sink    *RotatingFile
	encoder *json.Encoder
}

func NewJsonLogWriter(directory, unitName string, maxSize int) (*JsonLogWriter, error) {
	sink, err := NewRotatingFile(directory, unitName, maxSize)
	if err != nil {
		return nil, err
	}
	encoder := json.NewEncoder(sink)
	logger := &JsonLogWriter{
		sink:    sink,
		encoder: encoder,
	}
	return logger, nil
}

// Todo, consider turning this into an io.Writer by not using the encoder
func (o JsonLogWriter) Write(entry LogEntry) error {
	return o.encoder.Encode(entry)
}
