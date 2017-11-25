package server

import "sync"

type StringMap struct {
	sync.RWMutex
	data map[string]string
}

func (m *StringMap) Add(key, value string) {
	m.Lock()
	m.data[key] = value
	m.Unlock()
}

func (m *StringMap) Delete(key string) {
	m.Lock()
	delete(m.data, key)
	m.Unlock()
}

func (m *StringMap) Get(key string) (string, bool) {
	m.RLock()
	value, exists := m.data[key]
	m.RUnlock()
	return value, exists
}

func (m *StringMap) Items() [][2]string {
	m.RLock()
	items := make([][2]string, 0, len(m.data))
	for k, v := range m.data {
		d := [2]string{k, v}
		items = append(items, d)
	}
	return items
}
