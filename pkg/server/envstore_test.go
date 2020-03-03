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
	for _, item := range expected {
		assert.Contains(t, items, item)
	}
	e.Delete("foo", "name2")
	items = e.Items("foo")
	assert.Len(t, items, 1)
	assert.Equal(t, expected[0], items[0])
}
