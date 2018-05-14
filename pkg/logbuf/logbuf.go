package logbuf

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

func minint64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (lb *LogBuffer) GetOffset() int64 {
	return lb.offset
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
		n = minint64(lb.capacity, lb.offset)
	}
	entries := make([]LogEntry, n)
	// Xibit: Yo dawg, I heard you like off-by-one-errors so I put an
	// off-by one-error in your off-by-one-error so you can fuck up
	// while your're fucking up, while you're fucking up, while you're
	// fucking up.
	//
	// I did test this on paper...
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
	if i > offset {
		return entries, offset
	} else if i == offset {
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
