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
	})
}

// makeMapVal builds a MapVal with a single key-value pair for test brevity.
func makeMapVal(key string, val provider.Value) provider.Value {
	m := provider.NewOrderedMap()
	m.Set(key, val)
	return provider.MapVal(m)
}
