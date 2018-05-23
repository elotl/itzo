package conmap

import (
	"sync"

	"github.com/justnoise/genny/generic"
)

// Mostly taken from
// https://github.com/cheekybits/gennylib/blob/master/maps/concurrentmap.go
// We've added an Items() function. Don't use cheekybits/genny since
// that doesn't have a good golang parser, use the one from justnoise
// (that's ME!)

//go:generate genny -in=$GOFILE -out=gen-$GOFILE gen "KeyType=string ValueType=string,*os.Process,*logbuf.LogBuffer"

type KeyType generic.Type

type ValueType generic.Type

type KeyTypeValueType struct {
	sync.RWMutex
	data map[KeyType]ValueType
}

type NodeKeyTypeValueType struct {
	Key   KeyType
	Value ValueType
}

func NewKeyTypeValueType() *KeyTypeValueType {
	return &KeyTypeValueType{
		data: make(map[KeyType]ValueType),
	}
}

func (m *KeyTypeValueType) Set(key KeyType, value ValueType) {
	m.Lock()
	m.data[key] = value
	m.Unlock()
}

func (m *KeyTypeValueType) Delete(key KeyType) {
	m.Lock()
	delete(m.data, key)
	m.Unlock()
}

func (m *KeyTypeValueType) Get(key KeyType) ValueType {
	m.RLock()
	value := m.data[key]
	m.RUnlock()
	return value
}

func (m *KeyTypeValueType) GetOK(key KeyType) (ValueType, bool) {
	m.RLock()
	value, exists := m.data[key]
	m.RUnlock()
	return value, exists
}

func (m *KeyTypeValueType) Len() int {
	m.RLock()
	len := len(m.data)
	m.RUnlock()
	return len
}

func (m *KeyTypeValueType) Items() []NodeKeyTypeValueType {
	m.RLock()
	items := make([]NodeKeyTypeValueType, 0, len(m.data))
	for k, v := range m.data {
		items = append(items, NodeKeyTypeValueType{Key: k, Value: v})
	}
	m.RUnlock()
	return items
}
