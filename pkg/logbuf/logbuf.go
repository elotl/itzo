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

package logbuf

import (
	"fmt"
	"time"

	"github.com/elotl/itzo/pkg/util"
)

const (
	StdoutLogSource LogSource = "stdout"
	StderrLogSource LogSource = "stderr"
	HelperLogSource LogSource = "helper"
)

type LogSource string

type LogEntry struct {
	Timestamp string
	Source    LogSource
	Line      string
}

func (le *LogEntry) Format(withMetadata bool) string {
	if !withMetadata {
		return le.Line
	}
	// Since we read our log lines line-by-line and have no way
	// to determine if the current line is a continuation of the
	// previous line, our tag is always "F" for a full line. The
	// other known tag is "P" for partial
	tags := "F"
	line := le.Line
	if line[len(line)-1] != '\n' {
		line += "\n"
	}
	return fmt.Sprintf(
		"%s %s %s %s",
		le.Timestamp,
		string(le.Source),
		tags,
		le.Line,
	)
}

// Lets play a fast and loose with our locking...  There isn't a lot of
// contention for writing and reading is pretty light too.
type LogBuffer struct {
	buf      []LogEntry
	capacity int64
	offset   int64 // Points to the next place we're going to write, increment only
}

func NewLogBuffer(capacity int) *LogBuffer {
	lb := LogBuffer{
		buf:      make([]LogEntry, capacity),
		capacity: int64(capacity),
	}
	return &lb
}

func (lb *LogBuffer) GetOffset() int64 {
	return lb.offset
}

func (lb *LogBuffer) Write(source LogSource, line string, timestamp *string) {
	Timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	if timestamp != nil {
		Timestamp = *timestamp
	}
	e := LogEntry{
		Timestamp: Timestamp,
		Source:    source,
		Line:      line,
	}
	bufLoc := lb.offset % lb.capacity
	lb.buf[bufLoc] = e
	lb.offset++
}

func (lb *LogBuffer) Length() int {
	return int(util.Minint64(lb.capacity, lb.offset))
}

func (lb *LogBuffer) Read(nn int) []LogEntry {
	offset := lb.offset
	n := int64(nn)
	if n > lb.capacity || n > lb.offset {
		n = 0
	}
	if n < 0 {
		return nil
	}
	if n == 0 {
		n = util.Minint64(lb.capacity, lb.offset)
	}
	entries := make([]LogEntry, n)
	// A little tricky with the indexing but I did walk through this
	// on paper...
	for i, j := int64(1), n-1; i <= n; i, j = i+1, j-1 {
		bufLoc := (offset - i) % lb.capacity
		entries[j] = lb.buf[bufLoc]
	}
	return entries
}

// Useful for following logs via a polling strategy
// Polling is easier to implement than an event system because
// we don't need to worry about creating a pumper to fan out
// entries
func (lb *LogBuffer) ReadSince(i int64) ([]LogEntry, int64) {
	offset := lb.offset
	nRead := int64(0)
	entries := []LogEntry{}
	if i >= offset {
		return entries, offset
	}

	// if i is so far in the past that we're more than logBufSize
	// behind then skip forward until we're caught up with the current
	// buffer
	if i+lb.capacity < offset {
		i = offset - lb.capacity
	}

	// entries can wrap around to the start of the buffer so skipt the
	// slice tricks, iterate through everything.
	entries = make([]LogEntry, 0, offset-i)
	for ; i < offset; i++ {
		bufLoc := i % lb.capacity
		nRead++
		entries = append(entries, lb.buf[bufLoc])
	}

	// returns string, lines read, current offset
	return entries, offset
}

func (lb *LogBuffer) flush() {
	lb.buf = make([]LogEntry, lb.capacity)
	lb.offset = 0
}
