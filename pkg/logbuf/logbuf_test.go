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

func TestLogBufferFormat(t *testing.T) {
	msg := "line one\n"
	lb := NewLogBuffer(5)
	lb.Write(StderrLogSource, msg)
	entries := lb.Read(2)
	if len(entries) != 1 {
		t.FailNow()
	}
	ts := entries[0].Timestamp
	logOutput := entries[0].Format(true)
	expected := fmt.Sprintf("%s %s F line one\n", ts, string(StderrLogSource))
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
