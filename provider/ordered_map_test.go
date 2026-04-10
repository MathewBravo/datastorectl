package provider

import "testing"

func TestOrderedMapGet(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", IntVal(1))
	m.Set("b", StringVal("hello"))

	tests := []struct {
		name    string
		key     string
		wantVal Value
		wantOK  bool
	}{
		{"hit_first", "a", IntVal(1), true},
		{"hit_second", "b", StringVal("hello"), true},
		{"miss", "z", Value{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := m.Get(tt.key)
			if ok != tt.wantOK {
				t.Fatalf("Get(%q) ok = %v, want %v", tt.key, ok, tt.wantOK)
			}
			if !got.Equal(tt.wantVal) {
				t.Errorf("Get(%q) = %v, want %v", tt.key, got, tt.wantVal)
			}
		})
	}

	t.Run("after_update", func(t *testing.T) {
		m.Set("a", IntVal(99))
		got, ok := m.Get("a")
		if !ok || got.Int != 99 {
			t.Errorf("Get after update = (%v, %v), want (99, true)", got, ok)
		}
	})

	t.Run("empty_map", func(t *testing.T) {
		empty := NewOrderedMap()
		_, ok := empty.Get("x")
		if ok {
			t.Error("Get on empty map should return false")
		}
	})
}

func TestOrderedMapDelete(t *testing.T) {
	t.Run("delete_middle", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		m.Set("b", IntVal(2))
		m.Set("c", IntVal(3))
		m.Delete("b")

		if m.Len() != 2 {
			t.Fatalf("Len() = %d, want 2", m.Len())
		}
		keys := m.Keys()
		if keys[0] != "a" || keys[1] != "c" {
			t.Errorf("Keys() = %v, want [a c]", keys)
		}
	})

	t.Run("delete_first", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		m.Set("b", IntVal(2))
		m.Delete("a")

		keys := m.Keys()
		if len(keys) != 1 || keys[0] != "b" {
			t.Errorf("Keys() = %v, want [b]", keys)
		}
	})

	t.Run("delete_last", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		m.Set("b", IntVal(2))
		m.Delete("b")

		keys := m.Keys()
		if len(keys) != 1 || keys[0] != "a" {
			t.Errorf("Keys() = %v, want [a]", keys)
		}
	})

	t.Run("delete_missing_noop", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		m.Delete("z")
		if m.Len() != 1 {
			t.Errorf("Len() = %d, want 1 (no-op delete)", m.Len())
		}
	})

	t.Run("delete_from_empty", func(t *testing.T) {
		m := NewOrderedMap()
		m.Delete("x") // should not panic
		if m.Len() != 0 {
			t.Errorf("Len() = %d, want 0", m.Len())
		}
	})

	t.Run("order_preserved", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		m.Set("b", IntVal(2))
		m.Set("c", IntVal(3))
		m.Set("d", IntVal(4))
		m.Delete("b")

		want := []string{"a", "c", "d"}
		keys := m.Keys()
		if len(keys) != len(want) {
			t.Fatalf("Keys() length = %d, want %d", len(keys), len(want))
		}
		for i, k := range keys {
			if k != want[i] {
				t.Errorf("Keys()[%d] = %q, want %q", i, k, want[i])
			}
		}
	})
}

func TestOrderedMapKeys(t *testing.T) {
	t.Run("correct_order", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("c", IntVal(3))
		m.Set("a", IntVal(1))
		m.Set("b", IntVal(2))

		keys := m.Keys()
		want := []string{"c", "a", "b"}
		if len(keys) != len(want) {
			t.Fatalf("len = %d, want %d", len(keys), len(want))
		}
		for i, k := range keys {
			if k != want[i] {
				t.Errorf("Keys()[%d] = %q, want %q", i, k, want[i])
			}
		}
	})

	t.Run("returned_slice_is_copy", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		m.Set("b", IntVal(2))

		keys := m.Keys()
		keys[0] = "MUTATED"

		original := m.Keys()
		if original[0] != "a" {
			t.Errorf("mutating Keys() result affected map: got %q, want %q", original[0], "a")
		}
	})

	t.Run("nil_map", func(t *testing.T) {
		var m *OrderedMap
		keys := m.Keys()
		if keys != nil {
			t.Errorf("nil map Keys() = %v, want nil", keys)
		}
	})

	t.Run("empty_map", func(t *testing.T) {
		m := NewOrderedMap()
		keys := m.Keys()
		if len(keys) != 0 {
			t.Errorf("empty map Keys() len = %d, want 0", len(keys))
		}
	})
}

func TestOrderedMapEqual(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("a", IntVal(1))
	m1.Set("b", IntVal(2))

	m2 := NewOrderedMap()
	m2.Set("a", IntVal(1))
	m2.Set("b", IntVal(2))

	mDiffVal := NewOrderedMap()
	mDiffVal.Set("a", IntVal(1))
	mDiffVal.Set("b", IntVal(99))

	mDiffOrder := NewOrderedMap()
	mDiffOrder.Set("b", IntVal(2))
	mDiffOrder.Set("a", IntVal(1))

	mDiffLen := NewOrderedMap()
	mDiffLen.Set("a", IntVal(1))

	tests := []struct {
		name string
		a, b *OrderedMap
		want bool
	}{
		{"same_maps", m1, m2, true},
		{"different_values", m1, mDiffVal, false},
		{"different_order", m1, mDiffOrder, false},
		{"different_lengths", m1, mDiffLen, false},
		{"both_nil", nil, nil, true},
		{"nil_vs_empty", nil, NewOrderedMap(), true},
		{"empty_vs_nil", NewOrderedMap(), nil, true},
		{"one_nil", nil, m1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderedMapClone(t *testing.T) {
	t.Run("clone_equals_original", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		m.Set("b", StringVal("hello"))

		c := m.Clone()
		if !m.Equal(c) {
			t.Error("cloned map should equal original")
		}
	})

	t.Run("mutating_clone_does_not_affect_original", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		m.Set("b", IntVal(2))

		c := m.Clone()
		c.Set("a", IntVal(99))
		c.Delete("b")
		c.Set("c", IntVal(3))

		// Original should be unchanged.
		v, ok := m.Get("a")
		if !ok || v.Int != 1 {
			t.Errorf("original a = (%v, %v), want (1, true)", v, ok)
		}
		if m.Len() != 2 {
			t.Errorf("original Len() = %d, want 2", m.Len())
		}
	})

	t.Run("clone_with_nested_map", func(t *testing.T) {
		inner := NewOrderedMap()
		inner.Set("x", IntVal(10))

		m := NewOrderedMap()
		m.Set("nested", MapVal(inner))

		c := m.Clone()

		// Mutate the inner map of the clone.
		nestedClone, _ := c.Get("nested")
		nestedClone.Map.Set("x", IntVal(999))

		// Original inner map should be unaffected.
		nestedOrig, _ := m.Get("nested")
		v, _ := nestedOrig.Map.Get("x")
		if v.Int != 10 {
			t.Errorf("nested original x = %d, want 10", v.Int)
		}
	})

	t.Run("clone_with_nested_list", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("items", ListVal([]Value{IntVal(1), IntVal(2)}))

		c := m.Clone()

		// Mutate the list in the clone.
		itemsClone, _ := c.Get("items")
		itemsClone.List[0] = IntVal(999)

		// Original list should be unaffected.
		itemsOrig, _ := m.Get("items")
		if itemsOrig.List[0].Int != 1 {
			t.Errorf("original list[0] = %d, want 1", itemsOrig.List[0].Int)
		}
	})

	t.Run("nil_clone", func(t *testing.T) {
		var m *OrderedMap
		c := m.Clone()
		if c != nil {
			t.Error("nil.Clone() should return nil")
		}
	})
}
