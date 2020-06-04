package containerlog

import (
	"encoding/json"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

type LogStream string

const (
	Stdout LogStream = "stdout"
	Stderr LogStream = "stderr"
)

type Logger struct {
	lumberjack lumberjack.Logger
	attrs      map[string]string
}

// JSONLog is the Docker log message format.
type JSONLog struct {
	// Log contains the log message.
	Log string `json:"log"`
	// Stream is source, such as stdout or stderr.
	Stream string `json:"stream,omitempty"`
	// Created is the timestamp.
	Created time.Time `json:"time"`
	// Attrs is a map extra attributes that will be added to entries.
	Attrs map[string]string `json:"attrs,omitempty"`
}

func NewLogger(filename string, maxSize, maxBackups, maxAge int, attrs map[string]string) *Logger {
	attrsCopy := make(map[string]string)
	for k, v := range attrs {
		attrsCopy[k] = v
	}
	return &Logger{
		lumberjack: lumberjack.Logger{
			Filename:   filename,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			MaxAge:     maxAge, // days
			Compress:   false,
		},
		attrs: attrsCopy,
	}
}

func (l *Logger) Write(stream LogStream, line string) error {
	entry := JSONLog{
		Log:     line,
		Stream:  string(stream),
		Created: time.Now(),
		Attrs:   l.attrs,
	}
	buf, err := json.Marshal(&entry)
	if err != nil {
		return err
	}
	_, err = l.lumberjack.Write(append(buf, '\n'))
	return err
}
