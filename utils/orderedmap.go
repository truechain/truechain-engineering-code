package utils

import (
	"container/list"
	"fmt"
)

// Provides a common ordered map.
type OrderedMap struct {
	m map[interface{}]interface{}
	l *list.List
}

func NewOrderedMap(items ...interface{}) *OrderedMap {
	s := &OrderedMap{}
	s.m = make(map[interface{}]interface{})
	s.l = list.New()

	return s
}

func (om *OrderedMap) Set(key interface{}, value interface{}) {
	if _, ok := om.m[key]; ok == false {
		om.l.PushBack(key)
	}
	om.m[key] = value
}

func (om *OrderedMap) Get(key interface{}) (interface{}, bool) {
	val, ok := om.m[key]
	return val, ok
}

func (om *OrderedMap) Delete(key interface{}) {
	_, ok := om.m[key]
	if ok {
		delete(om.m, key)
	}

	for e := om.l.Front(); e != nil; e = e.Next() {
		if e.Value == key {
			om.l.Remove(e)
			break
		}
	}
}

func (om *OrderedMap) String() string {
	builder := make([]string, len(om.m))

	var index int = 0
	for e := om.l.Front(); e != nil; e = e.Next() {
		key := e.Value
		val, _ := om.m[key]

		builder[index] = fmt.Sprintf("%v:%v, ", key, val)
		index++
	}

	return fmt.Sprintf("OrderedMap%v", builder)
}

// Pop  deletes and return item from the key,value pair. The underlying map is
// modified. If map is empty, nil is returned.
func (om *OrderedMap) Pop() (interface{}, interface{}) {
	if e := om.l.Front(); e != nil {
		key := e.Value
		value := om.m[key]
		delete(om.m, key)

		om.l.Remove(e)

		return key, value
	}

	return nil, nil
}

// Has looks for the existence of items passed. It returns false if nothing is
// passed. For multiple items it returns true only if all of  the items exist.
func (om *OrderedMap) Has(items ...interface{}) bool {
	// assume checked for empty item, which not exist
	if len(items) == 0 {
		return false
	}

	has := true
	for _, item := range items {
		if _, has = om.m[item]; !has {
			break
		}
	}
	return has
}

// Size returns the number of items in a map.
func (om *OrderedMap) Size() int {
	return len(om.m)
}

// Clear removes all items from the map.
func (om *OrderedMap) Clear() {
	om.m = make(map[interface{}]interface{})
	om.l = list.New()
}

// IsEmpty reports whether the map is empty.
func (om *OrderedMap) IsEmpty() bool {
	return om.Size() == 0
}
