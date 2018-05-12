package server

import (
	"time"
)

type LogEntry struct {
	Timestamp string
	Source    string
	Line      string
}

// Lets play a fast and loose with our locking...  There isn't a lot of
// contention for writing and reading is pretty light too.
type LogBuffer struct {
	buf      []LogEntry
	capacity int64
	offset   int64
}

func NewLogBuffer(capacity int) *LogBuffer {
	lb := LogBuffer{
		buf:      make([]LogEntry, capacity),
		capacity: int64(capacity),
	}
	return &lb
}

func (lb *LogBuffer) Write(source, line string) {
	e := LogEntry{
		Timestamp: time.Now().String(),
		Source:    source,
		Line:      line,
	}
	bufLoc := lb.offset % lb.capacity
	lb.buf[bufLoc] = e
	lb.offset++
}

func (lb *LogBuffer) Length() int {
	return int(minint64(lb.capacity, lb.offset))
}

// Useful for following logs via a polling strategy
// Polling is easier to implement than an event system because
// we don't need to worry about creating a pumper to fan out
// entries
func (lb *LogBuffer) ReadSince(i int64) ([]LogEntry, int64, int64) {
	offset := lb.offset
	nRead := int64(0)
	entries := []LogEntry{}
	if i > offset {
		return entries, nRead, offset
	} else if i == offset {
		return entries, nRead, offset
	}

	// if i is so far in the past that we're more than logBufSize
	// behind then skip forward until we're caught up with the current
	// buffer
	for ; i+lb.capacity < offset; i += lb.capacity {
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
	return entries, nRead, offset
}

func (lb *LogBuffer) Read(n int64) []LogEntry {
	offset := lb.offset
	if n > lb.capacity || n > lb.offset {
		n = 0
	}
	if n < 0 {
		return nil
	}
	if n == 0 {
		n = minint64(lb.capacity, lb.offset)
	}
	entries := make([]LogEntry, n, n)
	// Xibit: I heard you like off-by-one-errors! We put some off-by
	// one-errors in your off-by-one-errors so you can fuck up while
	// your're fucking up, while you're fucking up, while you're
	// fucking up.
	for i, j := int64(1), n-1; i <= n; i, j = i+1, j-1 {
		bufLoc := (offset - i) % lb.capacity
		entries[j] = lb.buf[bufLoc]
	}
	return entries
}

func (lb *LogBuffer) flush() {
	lb.buf = make([]LogEntry, lb.capacity)
}
