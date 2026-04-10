package provider

// OrderedMap is a string-keyed map that preserves insertion order.
// Iteration order is semantically significant — two maps with the same
// keys in different order are not equal.
type OrderedMap struct {
	keys   []string
	values []Value
}

// NewOrderedMap returns an empty OrderedMap.
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{}
}

// Set adds or updates a key-value pair. If key already exists, the value
// is updated in place without changing the key's position.
func (m *OrderedMap) Set(key string, v Value) {
	for i, k := range m.keys {
		if k == key {
			m.values[i] = v
			return
		}
	}
	m.keys = append(m.keys, key)
	m.values = append(m.values, v)
}

// Len returns the number of entries. It is nil-safe.
func (m *OrderedMap) Len() int {
	if m == nil {
		return 0
	}
	return len(m.keys)
}
