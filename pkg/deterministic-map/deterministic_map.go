package deterministicmap

import (
	"cmp"
	"errors"
	"sort"
)

// ErrBreak is a sentinel error that stops iteration gracefully.
var ErrBreak = errors.New("break iteration")

// entry represents a key/value pair.
type entry[K comparable, V any] struct {
	key   K
	value V
}

// Map is a deterministic map backed by a slice of entries.
// Iteration order is stable and deterministic.
type Map[K cmp.Ordered, V any] struct {
	index   map[K]int
	entries []entry[K, V]
}

// New creates an initialized deterministic Map.
// The zero value of Map is also safe to use.
func New[K cmp.Ordered, V any]() *Map[K, V] {
	return &Map[K, V]{
		index: make(map[K]int),
	}
}

// FromMap converts a native Go map into a deterministic Map.
// Keys are sorted once to establish canonical iteration order.
func FromMap[K cmp.Ordered, V any](src map[K]V) *Map[K, V] {
	m := &Map[K, V]{
		index:   make(map[K]int, len(src)),
		entries: make([]entry[K, V], 0, len(src)),
	}

	keys := make([]K, 0, len(src))
	for k := range src { //nolint:deterministicmaplint
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	for i, k := range keys {
		m.entries = append(m.entries, entry[K, V]{key: k, value: src[k]})
		m.index[k] = i
	}

	return m
}

// ensure initializes internal state for zero-value usage.
func (m *Map[K, V]) ensure() {
	if m.index == nil {
		m.index = make(map[K]int)
	}
}

// Set inserts or updates a key/value pair.
// New keys are appended deterministically.
func (m *Map[K, V]) Set(key K, value V) {
	m.ensure()

	if i, exists := m.index[key]; exists {
		m.entries[i].value = value
		return
	}

	m.index[key] = len(m.entries)
	m.entries = append(m.entries, entry[K, V]{key: key, value: value})
}

// Get retrieves a value by key.
func (m *Map[K, V]) Get(key K) (V, bool) {
	if m.index == nil {
		var zero V
		return zero, false
	}

	i, ok := m.index[key]
	if !ok {
		var zero V
		return zero, false
	}

	return m.entries[i].value, true
}

// Delete removes a key/value pair.
// Deletion is O(1) using swap-with-last, preserving deterministic iteration.
func (m *Map[K, V]) Delete(key K) {
	if m.index == nil {
		return
	}

	i, ok := m.index[key]
	if !ok {
		return
	}

	last := len(m.entries) - 1
	lastEntry := m.entries[last]

	if i != last {
		m.entries[i] = lastEntry
		m.index[lastEntry.key] = i
	}

	delete(m.index, key)
	m.entries = m.entries[:last]
}

// Len returns the number of entries in the map.
func (m *Map[K, V]) Len() int {
	if m.index == nil {
		return 0
	}
	return len(m.entries)
}

// Values returns the map values in deterministic iteration order.
func (m *Map[K, V]) Values() []V {
	if m.index == nil {
		return nil
	}

	out := make([]V, len(m.entries))
	for i, e := range m.entries {
		out[i] = e.value
	}
	return out
}

// Range iterates over the map in deterministic order.
// If the function returns ErrBreak, iteration stops gracefully.
// Any other error is propagated.
func (m *Map[K, V]) Range(fn func(key K, value V) error) error {
	if m.index == nil {
		return nil
	}

	for _, e := range m.entries {
		if err := fn(e.key, e.value); err != nil {
			if errors.Is(err, ErrBreak) {
				return nil
			}
			return err
		}
	}

	return nil
}
