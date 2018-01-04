package server

import (
	"sync"
	"time"
)

type LogEntry struct {
	Timestamp string
	Source    string
	Line      string
}

type LogBuffer struct {
	buf      []LogEntry
	capacity int
	lock     sync.RWMutex
}

func NewLogBuffer(capacity int) *LogBuffer {
	lb := LogBuffer{
		buf:      make([]LogEntry, 0, capacity),
		capacity: capacity,
	}
	return &lb
}

func (l *LogBuffer) Write(source, line string) {
	e := LogEntry{
		Timestamp: time.Now().String(),
		Source:    source,
		Line:      line,
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	l.buf = append(l.buf, e)
	if len(l.buf) > l.capacity {
		l.buf = l.buf[1:]
	}
}

func (l *LogBuffer) Length() int {
	l.lock.RLock()
	defer l.lock.RUnlock()
	return len(l.buf)
}

func (l *LogBuffer) Read(n int) []LogEntry {
	l.lock.RLock()
	defer l.lock.RUnlock()
	blen := len(l.buf)
	if n < 0 {
		return nil
	}
	if n == 0 {
		n = blen
	}
	if n > blen {
		n = blen
	}
	entries := make([]LogEntry, n, n)
	copy(entries, l.buf[blen-n:])
	return entries
}

func (l *LogBuffer) Flush() {
	l.buf = make([]LogEntry, 0, l.capacity)
}
