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

// Get returns the value for key and whether the key was found.
func (m *OrderedMap) Get(key string) (Value, bool) {
	for i, k := range m.keys {
		if k == key {
			return m.values[i], true
		}
	}
	return Value{}, false
}

// Delete removes a key and its value, preserving the order of remaining
// entries. It is a no-op if the key is not present.
func (m *OrderedMap) Delete(key string) {
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			m.values = append(m.values[:i], m.values[i+1:]...)
			return
		}
	}
}

// Keys returns a copy of the key slice. Callers may mutate the returned
// slice without affecting the map.
func (m *OrderedMap) Keys() []string {
	if m == nil {
		return nil
	}
	out := make([]string, len(m.keys))
	copy(out, m.keys)
	return out
}

// Equal reports whether m and other contain the same key-value pairs in the
// same order. It is nil-safe; two nil maps are equal, and nil is equal to
// an empty map.
func (m *OrderedMap) Equal(other *OrderedMap) bool {
	mLen := m.Len()
	oLen := other.Len()
	if mLen != oLen {
		return false
	}
	for i := 0; i < mLen; i++ {
		if m.keys[i] != other.keys[i] {
			return false
		}
		if !m.values[i].Equal(other.values[i]) {
			return false
		}
	}
	return true
}

// Clone returns a deep copy of the map. Nil receiver returns nil.
func (m *OrderedMap) Clone() *OrderedMap {
	if m == nil {
		return nil
	}
	out := &OrderedMap{
		keys:   make([]string, len(m.keys)),
		values: make([]Value, len(m.values)),
	}
	copy(out.keys, m.keys)
	for i, v := range m.values {
		out.values[i] = v.Clone()
	}
	return out
}
