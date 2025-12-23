package deterministicmap

import (
	"cmp"
	"sort"
)

// Map is a deterministic, sorted map with lazy sorting.
// Iteration order is canonical and stable across executions.
type Map[K cmp.Ordered, V any] struct {
	data   map[K]V
	keys   []K
	sorted bool
}

// New creates an initialized sorted Map.
// The zero value of Map is also safe to use.
func New[K cmp.Ordered, V any]() *Map[K, V] {
	return &Map[K, V]{
		data:   make(map[K]V),
		sorted: true,
	}
}

// FromMap converts a native Go map into a deterministic Map.
// The resulting map has canonical sorted order.
func FromMap[K cmp.Ordered, V any](src map[K]V) *Map[K, V] {
	m := &Map[K, V]{
		data:   make(map[K]V, len(src)),
		keys:   make([]K, 0, len(src)),
		sorted: false,
	}

	for k, v := range src { //nolint:deterministicmaplint
		m.data[k] = v
		m.keys = append(m.keys, k)
	}

	return m
}

// ensure initializes internal state for zero-value usage.
func (m *Map[K, V]) ensure() {
	if m.data == nil {
		m.data = make(map[K]V)
		m.sorted = true
	}
}

// Set inserts or updates a key/value pair.
// Insertion of a new key invalidates sort order.
func (m *Map[K, V]) Set(key K, value V) {
	m.ensure()

	if _, exists := m.data[key]; !exists {
		m.keys = append(m.keys, key)
		m.sorted = false
	}

	m.data[key] = value
}

// Get retrieves a value by key.
func (m *Map[K, V]) Get(key K) (V, bool) {
	if m.data == nil {
		var zero V
		return zero, false
	}
	v, ok := m.data[key]
	return v, ok
}

// Delete removes a key/value pair.
// Deletion invalidates sort order.
func (m *Map[K, V]) Delete(key K) {
	if m.data == nil {
		return
	}

	if _, exists := m.data[key]; !exists {
		return
	}

	delete(m.data, key)

	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}

	m.sorted = false
}

// Len returns the number of entries in the map.
func (m *Map[K, V]) Len() int {
	if m.data == nil {
		return 0
	}
	return len(m.keys)
}

// ensureSorted sorts keys if the map is marked as unsorted.
func (m *Map[K, V]) ensureSorted() {
	if m.sorted {
		return
	}
	sort.Slice(m.keys, func(i, j int) bool {
		return m.keys[i] < m.keys[j]
	})
	m.sorted = true
}

// Keys returns all keys in deterministic sorted order.
func (m *Map[K, V]) Keys() []K {
	if m.data == nil {
		return nil
	}

	m.ensureSorted()

	out := make([]K, len(m.keys))
	copy(out, m.keys)
	return out
}

// Values returns all values in deterministic key order.
func (m *Map[K, V]) Values() []V {
	if m.data == nil {
		return nil
	}

	m.ensureSorted()

	out := make([]V, 0, len(m.keys))
	for _, k := range m.keys {
		out = append(out, m.data[k])
	}
	return out
}

// Range iterates over the map in deterministic sorted order.
// Returning false from fn stops iteration.
func (m *Map[K, V]) Range(fn func(key K, value V) bool) {
	if m.data == nil {
		return
	}

	m.ensureSorted()

	for _, k := range m.keys {
		if !fn(k, m.data[k]) {
			return
		}
	}
}

// RangeErr iterates over the map in deterministic sorted order.
// Returning error from fn stops iteration and returns the error.
func (m *Map[K, V]) RangeErr(fn func(key K, value V) error) error {
	if m.data == nil {
		return nil
	}

	m.ensureSorted()

	for _, k := range m.keys {
		if err := fn(k, m.data[k]); err != nil {
			return err
		}
	}

	return nil
}
