package logbuf

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mklogsrc(format string, a ...interface{}) LogSource {
	return LogSource(fmt.Sprintf(format, a...))
}

func TestLogBufferWrapAround(t *testing.T) {
	lb := NewLogBuffer(3)
	for i := 0; i < 3; i++ {
		lb.Write(mklogsrc("src %d", i+1), fmt.Sprintf("line %d", i+1))
		assert.Equal(t, i+1, lb.Length())
	}
	for i := 0; i < 5; i++ {
		lb.Write(mklogsrc("src %d", i+4), fmt.Sprintf("line %d", i+4))
		assert.Equal(t, 3, lb.Length())
	}
}

func TestLogBufferNewlines(t *testing.T) {
	lb := NewLogBuffer(10)
	cases := []struct {
		msg   string
		lines int
		info  string
	}{
		{
			msg:   "\n",
			lines: 1,
			info:  "Empty newline should log",
		},
		{
			msg:   "first line %d\n",
			lines: 1,
			info:  "Trailing newlines shouldnt add a new line",
		},
		{
			msg:   "first line %d\nsecond line",
			lines: 2,
			info:  "multiline with no trailing newline",
		},
		{
			msg:   "first line\nsecond line with newline\n",
			lines: 2,
			info:  "multiline with trailing newline",
		},
	}
	for _, tc := range cases {
		lb.flush()
		lb.Write(StdoutLogSource, tc.msg)
		assert.Equal(t, tc.lines, lb.Length(), tc.info)
	}
}

func TestLogBufferPartialTag(t *testing.T) {
	lb := NewLogBuffer(10)
	lb.Write(StdoutLogSource, "one line")
	assert.Equal(t, 1, lb.Length())
	assert.False(t, lb.buf[0].Partial)
	lb.Write(StdoutLogSource, "two\nlines")
	assert.Equal(t, 3, lb.Length())
	assert.True(t, lb.buf[1].Partial)
	assert.False(t, lb.buf[2].Partial)
}

func TestLogBufferStringer(t *testing.T) {
	msg := "\nline one\nline two\n"
	lb := NewLogBuffer(5)
	lb.Write(StderrLogSource, msg)
	entries := lb.Read(2)
	if len(entries) != 2 {
		t.FailNow()
	}
	ts := entries[0].Timestamp
	logOutput := entries[0].String()
	logOutput += entries[1].String()
	expected := fmt.Sprintf(
		"%s %s P line one\n%s %s F line two\n",
		ts, string(StderrLogSource),
		ts, string(StderrLogSource),
	)
	assert.Equal(t, expected, logOutput)
}

func TestLogBufferOverflow(t *testing.T) {
	lb := NewLogBuffer(3)
	for i := 0; i < 5; i++ {
		lb.Write(mklogsrc("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries := lb.Read(3)
	assert.Equal(t, len(entries), 3)
	for i := 0; i < 3; i++ {
		src := mklogsrc("src %d", i+3)
		line := fmt.Sprintf("line %d", i+3)
		assert.Equal(t, src, entries[i].Source)
		assert.Equal(t, line, entries[i].Line)
		assert.NotEqual(t, nil, entries[i].Timestamp)
	}
}

func TestLogBufferReadAll(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 5; i++ {
		lb.Write(mklogsrc("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries := lb.Read(0)
	assert.Equal(t, len(entries), 5)
	for i := 0; i < 5; i++ {
		src := mklogsrc("src %d", i+1)
		line := fmt.Sprintf("line %d", i+1)
		assert.Equal(t, src, entries[i].Source)
		assert.Equal(t, line, entries[i].Line)
		assert.NotEqual(t, nil, entries[i].Timestamp)
	}
}

func TestLogBufferUnderRead(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 5; i++ {
		lb.Write(mklogsrc("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries := lb.Read(3)
	assert.Equal(t, len(entries), 3)
	for i := 0; i < 3; i++ {
		src := mklogsrc("src %d", i+3)
		line := fmt.Sprintf("line %d", i+3)
		assert.Equal(t, src, entries[i].Source)
		assert.Equal(t, line, entries[i].Line)
		assert.NotEqual(t, nil, entries[i].Timestamp)
	}
}

func TestLogBufferOverRead(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 5; i++ {
		lb.Write(mklogsrc("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries := lb.Read(15)
	assert.Equal(t, len(entries), 5)
	for i := 0; i < 5; i++ {
		src := mklogsrc("src %d", i+1)
		line := fmt.Sprintf("line %d", i+1)
		assert.Equal(t, src, entries[i].Source)
		assert.Equal(t, line, entries[i].Line)
		assert.NotEqual(t, nil, entries[i].Timestamp)
	}
}

func TestLogBufferNegativeRead(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 5; i++ {
		lb.Write(mklogsrc("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries := lb.Read(-1)
	assert.Equal(t, len(entries), 0)
}

func TestLogBufferReadSince(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 15; i++ {
		lb.Write(mklogsrc("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries, _ := lb.ReadSince(25)
	assert.Len(t, entries, 0)
	entries, _ = lb.ReadSince(15)
	assert.Len(t, entries, 0)
	entries, offset := lb.ReadSince(14)
	assert.Len(t, entries, 1)
	assert.Equal(t, int64(15), offset)

	entries, offset = lb.ReadSince(2)
	assert.Len(t, entries, 10)
	assert.Equal(t, int64(15), offset)
	for i, j := 5, 0; i < 15; i, j = i+1, j+1 {
		line := fmt.Sprintf("line %d", i+1)
		assert.Equal(t, entries[j].Line, line)
	}
}

func TestLogBufferFlush(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 5; i++ {
		lb.Write(mklogsrc("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	assert.Equal(t, 5, lb.Length())
	lb.flush()
	assert.Equal(t, 0, lb.Length())
}
