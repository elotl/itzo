package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// super basic, doesn't catch all the corners but it starts to work
// feel free to add more tests

func TestEnvStore(t *testing.T) {
	e := EnvStore{}
	e.Add("foo", "name1", "val1")
	e.Add("bar", "name1", "val2")
	v1, exists := e.Get("foo", "name1")
	assert.True(t, exists)
	v2, exists := e.Get("bar", "name1")
	assert.True(t, exists)
	assert.Equal(t, "val1", v1)
	assert.Equal(t, "val2", v2)
	e.Add("foo", "name2", "val2")
	items := e.Items("foo")
	assert.Len(t, items, 2)
	expected := [][2]string{
		{"name1", "val1"},
		{"name2", "val2"},
	}
	assert.Equal(t, expected, items)
	e.Delete("foo", "name2")
	items = e.Items("foo")
	assert.Len(t, items, 1)
	assert.Equal(t, expected[0], items[0])
}
