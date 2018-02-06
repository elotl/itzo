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
