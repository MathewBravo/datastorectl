package provider

import (
	"fmt"
	"testing"
)

func TestKindString(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{KindNull, "null"},
		{KindString, "string"},
		{KindInt, "int"},
		{KindFloat, "float"},
		{KindBool, "bool"},
		{KindList, "list"},
		{KindMap, "map"},
		{Kind(99), "Kind(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("Kind(%d).String() = %q, want %q", int(tt.kind), got, tt.want)
			}
		})
	}
}

func TestConstructors(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want Kind
	}{
		{"NullVal", NullVal(), KindNull},
		{"StringVal", StringVal("hello"), KindString},
		{"IntVal", IntVal(42), KindInt},
		{"FloatVal", FloatVal(3.14), KindFloat},
		{"BoolVal", BoolVal(true), KindBool},
		{"ListVal", ListVal([]Value{IntVal(1)}), KindList},
		{"MapVal", MapVal(NewOrderedMap()), KindMap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.val.Kind != tt.want {
				t.Errorf("%s().Kind = %v, want %v", tt.name, tt.val.Kind, tt.want)
			}
		})
	}
}

func TestConstructorValues(t *testing.T) {
	t.Run("StringVal", func(t *testing.T) {
		v := StringVal("hello")
		if v.Str != "hello" {
			t.Errorf("Str = %q, want %q", v.Str, "hello")
		}
	})
	t.Run("IntVal", func(t *testing.T) {
		v := IntVal(42)
		if v.Int != 42 {
			t.Errorf("Int = %d, want 42", v.Int)
		}
	})
	t.Run("FloatVal", func(t *testing.T) {
		v := FloatVal(3.14)
		if v.Float != 3.14 {
			t.Errorf("Float = %g, want 3.14", v.Float)
		}
	})
	t.Run("BoolVal", func(t *testing.T) {
		v := BoolVal(true)
		if !v.Bool {
			t.Errorf("Bool = false, want true")
		}
	})
	t.Run("ListVal", func(t *testing.T) {
		v := ListVal([]Value{IntVal(1), IntVal(2)})
		if len(v.List) != 2 {
			t.Errorf("len(List) = %d, want 2", len(v.List))
		}
	})
	t.Run("MapVal", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		v := MapVal(m)
		if v.Map.Len() != 1 {
			t.Errorf("Map.Len() = %d, want 1", v.Map.Len())
		}
	})
}

func TestEqual(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("a", IntVal(1))
	m1.Set("b", IntVal(2))

	m2 := NewOrderedMap()
	m2.Set("a", IntVal(1))
	m2.Set("b", IntVal(2))

	m3 := NewOrderedMap()
	m3.Set("b", IntVal(2))
	m3.Set("a", IntVal(1))

	m4 := NewOrderedMap()
	m4.Set("a", IntVal(1))
	m4.Set("b", IntVal(99))

	tests := []struct {
		name string
		a, b Value
		want bool
	}{
		// same kind, same value
		{"null==null", NullVal(), NullVal(), true},
		{"str==str", StringVal("hi"), StringVal("hi"), true},
		{"int==int", IntVal(7), IntVal(7), true},
		{"float==float", FloatVal(1.5), FloatVal(1.5), true},
		{"bool==bool", BoolVal(false), BoolVal(false), true},
		{"list==list", ListVal([]Value{IntVal(1)}), ListVal([]Value{IntVal(1)}), true},
		{"map==map", MapVal(m1), MapVal(m2), true},

		// same kind, different value
		{"str!=str", StringVal("a"), StringVal("b"), false},
		{"int!=int", IntVal(1), IntVal(2), false},
		{"float!=float", FloatVal(1.0), FloatVal(2.0), false},
		{"bool!=bool", BoolVal(true), BoolVal(false), false},
		{"list!=list_val", ListVal([]Value{IntVal(1)}), ListVal([]Value{IntVal(2)}), false},
		{"list!=list_len", ListVal([]Value{IntVal(1)}), ListVal([]Value{IntVal(1), IntVal(2)}), false},
		{"map!=map_val", MapVal(m1), MapVal(m4), false},
		{"map!=map_order", MapVal(m1), MapVal(m3), false},

		// cross-kind mismatches
		{"null!=str", NullVal(), StringVal(""), false},
		{"int!=float", IntVal(1), FloatVal(1.0), false},
		{"str!=int", StringVal("1"), IntVal(1), false},
		{"bool!=int", BoolVal(true), IntVal(1), false},
		{"list!=map", ListVal(nil), MapVal(NewOrderedMap()), false},

		// empty collections
		{"empty_list==empty_list", ListVal(nil), ListVal([]Value{}), true},
		{"empty_map==empty_map", MapVal(NewOrderedMap()), MapVal(NewOrderedMap()), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEqualNested(t *testing.T) {
	t.Run("list_of_maps", func(t *testing.T) {
		m1 := NewOrderedMap()
		m1.Set("x", IntVal(1))
		m2 := NewOrderedMap()
		m2.Set("x", IntVal(1))

		a := ListVal([]Value{MapVal(m1)})
		b := ListVal([]Value{MapVal(m2)})
		if !a.Equal(b) {
			t.Error("expected equal")
		}
	})

	t.Run("map_of_lists", func(t *testing.T) {
		m1 := NewOrderedMap()
		m1.Set("items", ListVal([]Value{StringVal("a"), StringVal("b")}))
		m2 := NewOrderedMap()
		m2.Set("items", ListVal([]Value{StringVal("a"), StringVal("b")}))

		if !MapVal(m1).Equal(MapVal(m2)) {
			t.Error("expected equal")
		}
	})

	t.Run("deep_inequality", func(t *testing.T) {
		m1 := NewOrderedMap()
		m1.Set("items", ListVal([]Value{StringVal("a")}))
		m2 := NewOrderedMap()
		m2.Set("items", ListVal([]Value{StringVal("b")}))

		if MapVal(m1).Equal(MapVal(m2)) {
			t.Error("expected not equal")
		}
	})
}

func TestValueString(t *testing.T) {
	m := NewOrderedMap()
	m.Set("name", StringVal("db"))
	m.Set("port", IntVal(9200))

	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"null", NullVal(), "null"},
		{"string", StringVal("hello"), `"hello"`},
		{"empty_string", StringVal(""), `""`},
		{"int", IntVal(42), "42"},
		{"negative_int", IntVal(-7), "-7"},
		{"float", FloatVal(3.14), "3.14"},
		{"bool_true", BoolVal(true), "true"},
		{"bool_false", BoolVal(false), "false"},
		{"list", ListVal([]Value{IntVal(1), StringVal("a")}), `[1, "a"]`},
		{"empty_list", ListVal(nil), "[]"},
		{"map", MapVal(m), `{name: "db", port: 9200}`},
		{"empty_map", MapVal(NewOrderedMap()), "{}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.val.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZeroValueIsNull(t *testing.T) {
	var v Value
	if v.Kind != KindNull {
		t.Errorf("zero Value.Kind = %v, want KindNull", v.Kind)
	}
	if s := v.String(); s != "null" {
		t.Errorf("zero Value.String() = %q, want %q", s, "null")
	}
	if !v.Equal(NullVal()) {
		t.Error("zero Value should Equal(NullVal())")
	}
}

func TestOrderedMapBasic(t *testing.T) {
	t.Run("new_is_empty", func(t *testing.T) {
		m := NewOrderedMap()
		if m.Len() != 0 {
			t.Errorf("Len() = %d, want 0", m.Len())
		}
	})

	t.Run("nil_len", func(t *testing.T) {
		var m *OrderedMap
		if m.Len() != 0 {
			t.Errorf("nil.Len() = %d, want 0", m.Len())
		}
	})

	t.Run("set_and_len", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		m.Set("b", IntVal(2))
		if m.Len() != 2 {
			t.Errorf("Len() = %d, want 2", m.Len())
		}
	})

	t.Run("set_update_in_place", func(t *testing.T) {
		m := NewOrderedMap()
		m.Set("a", IntVal(1))
		m.Set("a", IntVal(99))
		if m.Len() != 1 {
			t.Errorf("Len() = %d, want 1", m.Len())
		}
		if m.values[0].Int != 99 {
			t.Errorf("values[0].Int = %d, want 99", m.values[0].Int)
		}
	})
}

// Ensure String and Equal don't collide with fmt.Stringer assumptions.
var _ fmt.Stringer = Value{}
var _ fmt.Stringer = Kind(0)
