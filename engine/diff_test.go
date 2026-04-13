package engine

import (
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

func TestDiffValues_NoDiff(t *testing.T) {
	tests := []struct {
		name     string
		old, new provider.Value
	}{
		{"both_null", provider.NullVal(), provider.NullVal()},
		{"equal_strings", provider.StringVal("hello"), provider.StringVal("hello")},
		{"equal_ints", provider.IntVal(42), provider.IntVal(42)},
		{"equal_floats", provider.FloatVal(3.14), provider.FloatVal(3.14)},
		{"equal_bools", provider.BoolVal(true), provider.BoolVal(true)},
		{"equal_lists", provider.ListVal([]provider.Value{provider.IntVal(1), provider.IntVal(2)}),
			provider.ListVal([]provider.Value{provider.IntVal(1), provider.IntVal(2)})},
		{"equal_maps", makeMapVal("a", provider.IntVal(1)),
			makeMapVal("a", provider.IntVal(1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diffs := DiffValues("test", tt.old, tt.new)
			if len(diffs) != 0 {
				t.Fatalf("expected 0 diffs, got %d: %v", len(diffs), diffs)
			}
		})
	}
}

func TestDiffValues_Scalars(t *testing.T) {
	t.Run("added_from_null", func(t *testing.T) {
		diffs := DiffValues("field", provider.NullVal(), provider.StringVal("new"))
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d", len(diffs))
		}
		if diffs[0].Kind != DiffAdded {
			t.Fatalf("expected DiffAdded, got %s", diffs[0].Kind)
		}
		if diffs[0].Old.Kind != provider.KindNull {
			t.Fatalf("expected null Old, got %s", diffs[0].Old.Kind)
		}
		if diffs[0].New.Str != "new" {
			t.Fatalf("expected New=\"new\", got %q", diffs[0].New.Str)
		}
	})

	t.Run("removed_to_null", func(t *testing.T) {
		diffs := DiffValues("field", provider.StringVal("old"), provider.NullVal())
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d", len(diffs))
		}
		if diffs[0].Kind != DiffRemoved {
			t.Fatalf("expected DiffRemoved, got %s", diffs[0].Kind)
		}
		if diffs[0].Old.Str != "old" {
			t.Fatalf("expected Old=\"old\", got %q", diffs[0].Old.Str)
		}
		if diffs[0].New.Kind != provider.KindNull {
			t.Fatalf("expected null New, got %s", diffs[0].New.Kind)
		}
	})

	t.Run("string_modified", func(t *testing.T) {
		diffs := DiffValues("name", provider.StringVal("a"), provider.StringVal("b"))
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d", len(diffs))
		}
		if diffs[0].Kind != DiffModified {
			t.Fatalf("expected DiffModified, got %s", diffs[0].Kind)
		}
		if diffs[0].Old.Str != "a" || diffs[0].New.Str != "b" {
			t.Fatalf("expected Old=%q New=%q, got Old=%q New=%q", "a", "b", diffs[0].Old.Str, diffs[0].New.Str)
		}
	})

	t.Run("int_modified", func(t *testing.T) {
		diffs := DiffValues("count", provider.IntVal(1), provider.IntVal(2))
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d", len(diffs))
		}
		if diffs[0].Kind != DiffModified {
			t.Fatalf("expected DiffModified, got %s", diffs[0].Kind)
		}
		if diffs[0].Old.Int != 1 || diffs[0].New.Int != 2 {
			t.Fatalf("expected Old=1 New=2, got Old=%d New=%d", diffs[0].Old.Int, diffs[0].New.Int)
		}
	})

	t.Run("float_modified", func(t *testing.T) {
		diffs := DiffValues("ratio", provider.FloatVal(1.0), provider.FloatVal(2.0))
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d", len(diffs))
		}
		if diffs[0].Kind != DiffModified {
			t.Fatalf("expected DiffModified, got %s", diffs[0].Kind)
		}
	})

	t.Run("bool_modified", func(t *testing.T) {
		diffs := DiffValues("enabled", provider.BoolVal(true), provider.BoolVal(false))
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d", len(diffs))
		}
		if diffs[0].Kind != DiffModified {
			t.Fatalf("expected DiffModified, got %s", diffs[0].Kind)
		}
	})

	t.Run("type_change", func(t *testing.T) {
		diffs := DiffValues("val", provider.StringVal("hello"), provider.IntVal(42))
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d", len(diffs))
		}
		if diffs[0].Kind != DiffModified {
			t.Fatalf("expected DiffModified, got %s", diffs[0].Kind)
		}
	})

	t.Run("path_preserved", func(t *testing.T) {
		diffs := DiffValues("metadata.team", provider.StringVal("a"), provider.StringVal("b"))
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d", len(diffs))
		}
		if diffs[0].Path != "metadata.team" {
			t.Fatalf("expected path %q, got %q", "metadata.team", diffs[0].Path)
		}
	})
}

func TestDiffValues_ComplexFallback(t *testing.T) {
	t.Run("different_lists", func(t *testing.T) {
		old := provider.ListVal([]provider.Value{provider.IntVal(1)})
		new := provider.ListVal([]provider.Value{provider.IntVal(2)})
		diffs := DiffValues("items", old, new)
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d", len(diffs))
		}
		if diffs[0].Kind != DiffModified {
			t.Fatalf("expected DiffModified, got %s", diffs[0].Kind)
		}
	})

	t.Run("different_maps", func(t *testing.T) {
		old := makeMapVal("a", provider.IntVal(1))
		new := makeMapVal("a", provider.IntVal(2))
		diffs := DiffValues("config", old, new)
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d", len(diffs))
		}
		if diffs[0].Kind != DiffModified {
			t.Fatalf("expected DiffModified, got %s", diffs[0].Kind)
		}
		if diffs[0].Path != "config.a" {
			t.Fatalf("expected path %q, got %q", "config.a", diffs[0].Path)
		}
	})
}

func TestDiffValues_Maps(t *testing.T) {
	t.Run("key_added", func(t *testing.T) {
		old := makeMapVal("a", provider.IntVal(1))
		nm := provider.NewOrderedMap()
		nm.Set("a", provider.IntVal(1))
		nm.Set("b", provider.IntVal(2))
		new := provider.MapVal(nm)
		diffs := DiffValues("root", old, new)
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d: %v", len(diffs), diffs)
		}
		if diffs[0].Kind != DiffAdded || diffs[0].Path != "root.b" {
			t.Fatalf("expected DiffAdded at root.b, got %s at %q", diffs[0].Kind, diffs[0].Path)
		}
	})

	t.Run("key_removed", func(t *testing.T) {
		om := provider.NewOrderedMap()
		om.Set("a", provider.IntVal(1))
		om.Set("b", provider.IntVal(2))
		old := provider.MapVal(om)
		new := makeMapVal("a", provider.IntVal(1))
		diffs := DiffValues("root", old, new)
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d: %v", len(diffs), diffs)
		}
		if diffs[0].Kind != DiffRemoved || diffs[0].Path != "root.b" {
			t.Fatalf("expected DiffRemoved at root.b, got %s at %q", diffs[0].Kind, diffs[0].Path)
		}
	})

	t.Run("value_modified", func(t *testing.T) {
		old := makeMapVal("a", provider.IntVal(1))
		new := makeMapVal("a", provider.IntVal(2))
		diffs := DiffValues("root", old, new)
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d: %v", len(diffs), diffs)
		}
		if diffs[0].Kind != DiffModified || diffs[0].Path != "root.a" {
			t.Fatalf("expected DiffModified at root.a, got %s at %q", diffs[0].Kind, diffs[0].Path)
		}
	})

	t.Run("multiple_changes", func(t *testing.T) {
		om := provider.NewOrderedMap()
		om.Set("a", provider.IntVal(1))
		om.Set("b", provider.IntVal(2))
		old := provider.MapVal(om)

		nm := provider.NewOrderedMap()
		nm.Set("a", provider.IntVal(3))
		nm.Set("c", provider.IntVal(4))
		new := provider.MapVal(nm)

		diffs := DiffValues("root", old, new)
		if len(diffs) != 3 {
			t.Fatalf("expected 3 diffs, got %d: %v", len(diffs), diffs)
		}
		// Old key order first: modified a, removed b.
		if diffs[0].Kind != DiffModified || diffs[0].Path != "root.a" {
			t.Fatalf("diffs[0]: expected DiffModified at root.a, got %s at %q", diffs[0].Kind, diffs[0].Path)
		}
		if diffs[1].Kind != DiffRemoved || diffs[1].Path != "root.b" {
			t.Fatalf("diffs[1]: expected DiffRemoved at root.b, got %s at %q", diffs[1].Kind, diffs[1].Path)
		}
		// Then new key order: added c.
		if diffs[2].Kind != DiffAdded || diffs[2].Path != "root.c" {
			t.Fatalf("diffs[2]: expected DiffAdded at root.c, got %s at %q", diffs[2].Kind, diffs[2].Path)
		}
	})

	t.Run("nested_map", func(t *testing.T) {
		old := makeMapVal("m", makeMapVal("x", provider.IntVal(1)))
		new := makeMapVal("m", makeMapVal("x", provider.IntVal(2)))
		diffs := DiffValues("root", old, new)
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d: %v", len(diffs), diffs)
		}
		if diffs[0].Kind != DiffModified || diffs[0].Path != "root.m.x" {
			t.Fatalf("expected DiffModified at root.m.x, got %s at %q", diffs[0].Kind, diffs[0].Path)
		}
	})

	t.Run("nested_map_key_added", func(t *testing.T) {
		old := makeMapVal("m", makeMapVal("x", provider.IntVal(1)))

		inner := provider.NewOrderedMap()
		inner.Set("x", provider.IntVal(1))
		inner.Set("y", provider.IntVal(2))
		new := makeMapVal("m", provider.MapVal(inner))

		diffs := DiffValues("root", old, new)
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d: %v", len(diffs), diffs)
		}
		if diffs[0].Kind != DiffAdded || diffs[0].Path != "root.m.y" {
			t.Fatalf("expected DiffAdded at root.m.y, got %s at %q", diffs[0].Kind, diffs[0].Path)
		}
	})

	t.Run("empty_to_populated", func(t *testing.T) {
		old := provider.MapVal(provider.NewOrderedMap())
		new := makeMapVal("a", provider.IntVal(1))
		diffs := DiffValues("root", old, new)
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d: %v", len(diffs), diffs)
		}
		if diffs[0].Kind != DiffAdded || diffs[0].Path != "root.a" {
			t.Fatalf("expected DiffAdded at root.a, got %s at %q", diffs[0].Kind, diffs[0].Path)
		}
	})

	t.Run("populated_to_empty", func(t *testing.T) {
		old := makeMapVal("a", provider.IntVal(1))
		new := provider.MapVal(provider.NewOrderedMap())
		diffs := DiffValues("root", old, new)
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d: %v", len(diffs), diffs)
		}
		if diffs[0].Kind != DiffRemoved || diffs[0].Path != "root.a" {
			t.Fatalf("expected DiffRemoved at root.a, got %s at %q", diffs[0].Kind, diffs[0].Path)
		}
	})

	t.Run("both_empty", func(t *testing.T) {
		old := provider.MapVal(provider.NewOrderedMap())
		new := provider.MapVal(provider.NewOrderedMap())
		diffs := DiffValues("root", old, new)
		if len(diffs) != 0 {
			t.Fatalf("expected 0 diffs, got %d: %v", len(diffs), diffs)
		}
	})

	t.Run("map_to_non_map", func(t *testing.T) {
		old := makeMapVal("a", provider.IntVal(1))
		new := provider.StringVal("not a map")
		diffs := DiffValues("root", old, new)
		if len(diffs) != 1 {
			t.Fatalf("expected 1 diff, got %d: %v", len(diffs), diffs)
		}
		if diffs[0].Kind != DiffModified {
			t.Fatalf("expected DiffModified, got %s", diffs[0].Kind)
		}
		if diffs[0].Path != "root" {
			t.Fatalf("expected path %q, got %q", "root", diffs[0].Path)
		}
	})
}

// makeMapVal builds a MapVal with a single key-value pair for test brevity.
func makeMapVal(key string, val provider.Value) provider.Value {
	m := provider.NewOrderedMap()
	m.Set(key, val)
	return provider.MapVal(m)
}
