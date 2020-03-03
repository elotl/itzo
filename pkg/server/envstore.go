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

import "sync"

// A 2d map protected by a mutex.
type EnvStore struct {
	sync.RWMutex
	data map[string]map[string]string
}

func (e *EnvStore) Add(unit, key, value string) {
	e.Lock()
	if e.data == nil {
		e.data = make(map[string]map[string]string)
	}
	m, exists := e.data[unit]
	if !exists {
		m = make(map[string]string)
		e.data[unit] = m
	}
	m[key] = value
	e.Unlock()
}

func (e *EnvStore) Delete(unit, key string) {
	e.Lock()
	if e.data != nil {
		m, exists := e.data[unit]
		if exists {
			delete(m, key)
		}
	}
	e.Unlock()
}

func (e *EnvStore) Get(unit, key string) (value string, exists bool) {
	e.Lock()
	value = ""
	exists = false
	if e.data != nil {
		m, unitExists := e.data[unit]
		if unitExists {
			value, exists = m[key]
		}
	}
	e.Unlock()
	return value, exists
}

func (e *EnvStore) Items(unit string) [][2]string {
	e.RLock()
	defer e.RUnlock()
	if e.data == nil {
		return make([][2]string, 0)
	}
	m, unitExists := e.data[unit]
	if !unitExists {
		return make([][2]string, 0)
	} else {
		items := make([][2]string, 0, len(m))
		for k, v := range m {
			d := [2]string{k, v}
			items = append(items, d)
		}
		return items
	}
}
