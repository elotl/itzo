package server

import (
	"fmt"
	"testing"
	//"time"

	"github.com/stretchr/testify/assert"
)

func TestLogBufferLength(t *testing.T) {
	lb := NewLogBuffer(3)
	for i := 0; i < 3; i++ {
		lb.Write(fmt.Sprintf("src %d", i+1), fmt.Sprintf("line %d", i+1))
		assert.Equal(t, i+1, lb.Length())
	}
	for i := 0; i < 5; i++ {
		lb.Write(fmt.Sprintf("src %d", i+4), fmt.Sprintf("line %d", i+4))
		assert.Equal(t, 3, lb.Length())
	}
}

func TestLogBufferOverflow(t *testing.T) {
	lb := NewLogBuffer(3)
	for i := 0; i < 5; i++ {
		lb.Write(fmt.Sprintf("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries := lb.Read(3)
	assert.Equal(t, len(entries), 3)
	for i := 0; i < 3; i++ {
		src := fmt.Sprintf("src %d", i+3)
		line := fmt.Sprintf("line %d", i+3)
		assert.Equal(t, src, entries[i].Source)
		assert.Equal(t, line, entries[i].Line)
		assert.NotEqual(t, nil, entries[i].Timestamp)
	}
}

func TestLogBufferReadAll(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 5; i++ {
		lb.Write(fmt.Sprintf("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries := lb.Read(0)
	assert.Equal(t, len(entries), 5)
	for i := 0; i < 5; i++ {
		src := fmt.Sprintf("src %d", i+1)
		line := fmt.Sprintf("line %d", i+1)
		assert.Equal(t, src, entries[i].Source)
		assert.Equal(t, line, entries[i].Line)
		assert.NotEqual(t, nil, entries[i].Timestamp)
	}
}

func TestLogBufferUnderRead(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 5; i++ {
		lb.Write(fmt.Sprintf("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries := lb.Read(3)
	assert.Equal(t, len(entries), 3)
	for i := 0; i < 3; i++ {
		src := fmt.Sprintf("src %d", i+3)
		line := fmt.Sprintf("line %d", i+3)
		assert.Equal(t, src, entries[i].Source)
		assert.Equal(t, line, entries[i].Line)
		assert.NotEqual(t, nil, entries[i].Timestamp)
	}
}

func TestLogBufferOverRead(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 5; i++ {
		lb.Write(fmt.Sprintf("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries := lb.Read(15)
	assert.Equal(t, len(entries), 5)
	for i := 0; i < 5; i++ {
		src := fmt.Sprintf("src %d", i+1)
		line := fmt.Sprintf("line %d", i+1)
		assert.Equal(t, src, entries[i].Source)
		assert.Equal(t, line, entries[i].Line)
		assert.NotEqual(t, nil, entries[i].Timestamp)
	}
}

func TestLogBufferNegativeRead(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 5; i++ {
		lb.Write(fmt.Sprintf("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	entries := lb.Read(-1)
	assert.Equal(t, len(entries), 0)
}

func TestLogBufferFlush(t *testing.T) {
	lb := NewLogBuffer(10)
	for i := 0; i < 5; i++ {
		lb.Write(fmt.Sprintf("src %d", i+1), fmt.Sprintf("line %d", i+1))
	}
	assert.Equal(t, 5, lb.Length())
	lb.flush()
	assert.Equal(t, 0, lb.Length())
}
