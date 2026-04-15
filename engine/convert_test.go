package engine

import (
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// --- ConvertFile tests ---

func TestConvertFile_Basic(t *testing.T) {
	t.Run("single_block", func(t *testing.T) {
		file := &dcl.File{
			Blocks: []dcl.Block{
				{
					Type:  "index",
					Label: "logs",
					Attributes: []dcl.Attribute{
						{Key: "replicas", Value: &dcl.LiteralInt{Value: 1}},
					},
				},
			},
		}
		rs, err := ConvertFile(file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rs.Resources) != 1 {
			t.Fatalf("len(Resources) = %d, want 1", len(rs.Resources))
		}
		r := rs.Resources[0]
		if r.ID.Type != "index" || r.ID.Name != "logs" {
			t.Errorf("ID = %v, want {index, logs}", r.ID)
		}
		wantBody := provider.NewOrderedMap()
		wantBody.Set("replicas", provider.IntVal(1))
		if !r.Body.Equal(wantBody) {
			t.Errorf("Body mismatch")
		}
	})

	t.Run("multiple_blocks", func(t *testing.T) {
		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "index", Label: "logs"},
				{Type: "template", Label: "t1"},
				{Type: "policy", Label: "p1"},
			},
		}
		rs, err := ConvertFile(file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rs.Resources) != 3 {
			t.Fatalf("len(Resources) = %d, want 3", len(rs.Resources))
		}
		wantIDs := []provider.ResourceID{
			{Type: "index", Name: "logs"},
			{Type: "template", Name: "t1"},
			{Type: "policy", Name: "p1"},
		}
		for i, want := range wantIDs {
			if rs.Resources[i].ID != want {
				t.Errorf("Resources[%d].ID = %v, want %v", i, rs.Resources[i].ID, want)
			}
		}
	})

	t.Run("empty_blocks", func(t *testing.T) {
		file := &dcl.File{Blocks: nil}
		rs, err := ConvertFile(file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rs == nil {
			t.Fatal("expected non-nil ResourceSet")
		}
		if len(rs.Resources) != 0 {
			t.Errorf("len(Resources) = %d, want 0", len(rs.Resources))
		}
	})

	t.Run("preserves_order", func(t *testing.T) {
		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "c_type", Label: "third"},
				{Type: "a_type", Label: "first"},
				{Type: "b_type", Label: "second"},
			},
		}
		rs, err := ConvertFile(file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rs.Resources) != 3 {
			t.Fatalf("len(Resources) = %d, want 3", len(rs.Resources))
		}
		if rs.Resources[0].ID.Type != "c_type" {
			t.Errorf("Resources[0].ID.Type = %q, want %q", rs.Resources[0].ID.Type, "c_type")
		}
		if rs.Resources[1].ID.Type != "a_type" {
			t.Errorf("Resources[1].ID.Type = %q, want %q", rs.Resources[1].ID.Type, "a_type")
		}
		if rs.Resources[2].ID.Type != "b_type" {
			t.Errorf("Resources[2].ID.Type = %q, want %q", rs.Resources[2].ID.Type, "b_type")
		}
	})
}

func TestConvertFile_Duplicates(t *testing.T) {
	t.Run("exact_duplicate", func(t *testing.T) {
		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "index", Label: "logs"},
				{Type: "index", Label: "logs"},
			},
		}
		_, err := ConvertFile(file, nil)
		if err == nil {
			t.Fatal("expected error for duplicate resource")
		}
		msg := err.Error()
		if !strings.Contains(msg, "duplicate resource") {
			t.Errorf("error = %q, want it to contain %q", msg, "duplicate resource")
		}
		if !strings.Contains(msg, "index.logs") {
			t.Errorf("error = %q, want it to contain %q", msg, "index.logs")
		}
	})

	t.Run("same_type_different_name", func(t *testing.T) {
		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "index", Label: "a"},
				{Type: "index", Label: "b"},
			},
		}
		rs, err := ConvertFile(file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rs.Resources) != 2 {
			t.Errorf("len(Resources) = %d, want 2", len(rs.Resources))
		}
	})

	t.Run("different_type_same_name", func(t *testing.T) {
		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "index", Label: "logs"},
				{Type: "template", Label: "logs"},
			},
		}
		rs, err := ConvertFile(file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rs.Resources) != 2 {
			t.Errorf("len(Resources) = %d, want 2", len(rs.Resources))
		}
	})

	t.Run("duplicate_among_many", func(t *testing.T) {
		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "index", Label: "logs"},
				{Type: "template", Label: "t1"},
				{Type: "policy", Label: "p1"},
				{Type: "index", Label: "logs"},
				{Type: "config", Label: "c1"},
			},
		}
		_, err := ConvertFile(file, nil)
		if err == nil {
			t.Fatal("expected error for duplicate resource")
		}
		msg := err.Error()
		if !strings.Contains(msg, "duplicate resource") {
			t.Errorf("error = %q, want it to contain %q", msg, "duplicate resource")
		}
	})
}

func TestConvertFile_Errors(t *testing.T) {
	t.Run("nil_file", func(t *testing.T) {
		_, err := ConvertFile(nil, nil)
		if err == nil {
			t.Fatal("expected error for nil file")
		}
		if !strings.Contains(err.Error(), "nil file") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "nil file")
		}
	})

	t.Run("file_with_parse_errors", func(t *testing.T) {
		file := &dcl.File{
			Diagnostics: dcl.Diagnostics{
				{Severity: dcl.SeverityError, Message: "unexpected token"},
			},
		}
		_, err := ConvertFile(file, nil)
		if err == nil {
			t.Fatal("expected error for file with parse errors")
		}
		if !strings.Contains(err.Error(), "parse errors") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "parse errors")
		}
	})

	t.Run("warnings_only_allowed", func(t *testing.T) {
		file := &dcl.File{
			Diagnostics: dcl.Diagnostics{
				{Severity: dcl.SeverityWarning, Message: "deprecated syntax"},
			},
			Blocks: []dcl.Block{
				{Type: "index", Label: "logs"},
			},
		}
		rs, err := ConvertFile(file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rs.Resources) != 1 {
			t.Errorf("len(Resources) = %d, want 1", len(rs.Resources))
		}
	})

	t.Run("block_conversion_error", func(t *testing.T) {
		file := &dcl.File{
			Blocks: []dcl.Block{
				{
					Type:  "index",
					Label: "bad",
					Attributes: []dcl.Attribute{
						{Key: "broken", Value: nil},
					},
				},
			},
		}
		_, err := ConvertFile(file, nil)
		if err == nil {
			t.Fatal("expected error for block conversion failure")
		}
		msg := err.Error()
		if !strings.Contains(msg, "block 0") {
			t.Errorf("error = %q, want it to contain %q", msg, "block 0")
		}
	})
}

func TestConvertFile_Fidelity(t *testing.T) {
	t.Run("bodies_converted", func(t *testing.T) {
		file := &dcl.File{
			Blocks: []dcl.Block{
				{
					Type:  "index",
					Label: "logs",
					Attributes: []dcl.Attribute{
						{Key: "replicas", Value: &dcl.LiteralInt{Value: 3}},
						{Key: "shards", Value: &dcl.LiteralInt{Value: 5}},
					},
				},
				{
					Type:  "template",
					Label: "t1",
					Attributes: []dcl.Attribute{
						{Key: "pattern", Value: &dcl.LiteralString{Value: "logs-*"}},
					},
				},
			},
		}
		rs, err := ConvertFile(file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantBody0 := provider.NewOrderedMap()
		wantBody0.Set("replicas", provider.IntVal(3))
		wantBody0.Set("shards", provider.IntVal(5))
		if !rs.Resources[0].Body.Equal(wantBody0) {
			t.Errorf("Resources[0].Body mismatch")
		}

		wantBody1 := provider.NewOrderedMap()
		wantBody1.Set("pattern", provider.StringVal("logs-*"))
		if !rs.Resources[1].Body.Equal(wantBody1) {
			t.Errorf("Resources[1].Body mismatch")
		}
	})

	t.Run("source_ranges_preserved", func(t *testing.T) {
		rng0 := dcl.Range{
			Start: dcl.Pos{Filename: "a.dcl", Line: 1, Column: 1},
			End:   dcl.Pos{Filename: "a.dcl", Line: 3, Column: 2},
		}
		rng1 := dcl.Range{
			Start: dcl.Pos{Filename: "a.dcl", Line: 5, Column: 1},
			End:   dcl.Pos{Filename: "a.dcl", Line: 8, Column: 2},
		}
		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "index", Label: "a", Rng: rng0},
				{Type: "index", Label: "b", Rng: rng1},
			},
		}
		rs, err := ConvertFile(file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rs.Resources[0].SourceRange != rng0 {
			t.Errorf("Resources[0].SourceRange = %v, want %v", rs.Resources[0].SourceRange, rng0)
		}
		if rs.Resources[1].SourceRange != rng1 {
			t.Errorf("Resources[1].SourceRange = %v, want %v", rs.Resources[1].SourceRange, rng1)
		}
	})
}

func TestExprToValue_Literals(t *testing.T) {
	tests := []struct {
		name string
		expr dcl.Expression
		want provider.Value
	}{
		{"string", &dcl.LiteralString{Value: "hello"}, provider.StringVal("hello")},
		{"empty_string", &dcl.LiteralString{Value: ""}, provider.StringVal("")},
		{"int", &dcl.LiteralInt{Value: 42}, provider.IntVal(42)},
		{"int_zero", &dcl.LiteralInt{Value: 0}, provider.IntVal(0)},
		{"int_negative", &dcl.LiteralInt{Value: -7}, provider.IntVal(-7)},
		{"float", &dcl.LiteralFloat{Value: 3.14}, provider.FloatVal(3.14)},
		{"float_zero", &dcl.LiteralFloat{Value: 0.0}, provider.FloatVal(0.0)},
		{"bool_true", &dcl.LiteralBool{Value: true}, provider.BoolVal(true)},
		{"bool_false", &dcl.LiteralBool{Value: false}, provider.BoolVal(false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exprToValue(tt.expr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExprToValue_Identifier(t *testing.T) {
	tests := []struct {
		name string
		expr dcl.Expression
		want provider.Value
	}{
		{"bare_word", &dcl.Identifier{Name: "standard"}, provider.StringVal("standard")},
		{"enum_value", &dcl.Identifier{Name: "primary"}, provider.StringVal("primary")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exprToValue(tt.expr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExprToValue_Reference(t *testing.T) {
	tests := []struct {
		name string
		expr dcl.Expression
		want provider.Value
	}{
		{"two_part", &dcl.Reference{Parts: []string{"db", "host"}}, provider.RefVal([]string{"db", "host"})},
		{"three_part", &dcl.Reference{Parts: []string{"db", "config", "port"}}, provider.RefVal([]string{"db", "config", "port"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exprToValue(tt.expr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExprToValue_FunctionCall(t *testing.T) {
	tests := []struct {
		name string
		expr dcl.Expression
		want provider.Value
	}{
		{
			"no_args",
			&dcl.FunctionCall{Name: "now"},
			provider.FuncCallVal("now", []provider.Value{}),
		},
		{
			"string_args",
			&dcl.FunctionCall{
				Name: "secret",
				Args: []dcl.Expression{
					&dcl.LiteralString{Value: "env"},
					&dcl.LiteralString{Value: "DB_PASS"},
				},
			},
			provider.FuncCallVal("secret", []provider.Value{
				provider.StringVal("env"),
				provider.StringVal("DB_PASS"),
			}),
		},
		{
			"mixed_arg_types",
			&dcl.FunctionCall{
				Name: "lookup",
				Args: []dcl.Expression{
					&dcl.LiteralString{Value: "table"},
					&dcl.LiteralInt{Value: 42},
				},
			},
			provider.FuncCallVal("lookup", []provider.Value{
				provider.StringVal("table"),
				provider.IntVal(42),
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exprToValue(tt.expr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExprToValue_List(t *testing.T) {
	tests := []struct {
		name string
		expr dcl.Expression
		want provider.Value
	}{
		{
			"empty",
			&dcl.ListExpr{},
			provider.ListVal([]provider.Value{}),
		},
		{
			"homogeneous_ints",
			&dcl.ListExpr{Elements: []dcl.Expression{
				&dcl.LiteralInt{Value: 1},
				&dcl.LiteralInt{Value: 2},
				&dcl.LiteralInt{Value: 3},
			}},
			provider.ListVal([]provider.Value{
				provider.IntVal(1),
				provider.IntVal(2),
				provider.IntVal(3),
			}),
		},
		{
			"mixed_types",
			&dcl.ListExpr{Elements: []dcl.Expression{
				&dcl.LiteralString{Value: "a"},
				&dcl.LiteralInt{Value: 1},
				&dcl.LiteralBool{Value: true},
			}},
			provider.ListVal([]provider.Value{
				provider.StringVal("a"),
				provider.IntVal(1),
				provider.BoolVal(true),
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exprToValue(tt.expr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExprToValue_Map(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		expr := &dcl.MapExpr{}
		got, err := exprToValue(expr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := provider.MapVal(provider.NewOrderedMap())
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("mixed_values", func(t *testing.T) {
		expr := &dcl.MapExpr{
			Keys:   []string{"name", "port", "enabled"},
			Values: []dcl.Expression{
				&dcl.LiteralString{Value: "db"},
				&dcl.LiteralInt{Value: 5432},
				&dcl.LiteralBool{Value: true},
			},
		}
		got, err := exprToValue(expr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := provider.NewOrderedMap()
		m.Set("name", provider.StringVal("db"))
		m.Set("port", provider.IntVal(5432))
		m.Set("enabled", provider.BoolVal(true))
		want := provider.MapVal(m)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("reference_as_value", func(t *testing.T) {
		expr := &dcl.MapExpr{
			Keys:   []string{"host"},
			Values: []dcl.Expression{
				&dcl.Reference{Parts: []string{"db", "host"}},
			},
		}
		got, err := exprToValue(expr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := provider.NewOrderedMap()
		m.Set("host", provider.RefVal([]string{"db", "host"}))
		want := provider.MapVal(m)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestExprToValue_Nested(t *testing.T) {
	t.Run("list_of_maps", func(t *testing.T) {
		expr := &dcl.ListExpr{Elements: []dcl.Expression{
			&dcl.MapExpr{
				Keys:   []string{"name"},
				Values: []dcl.Expression{&dcl.LiteralString{Value: "a"}},
			},
			&dcl.MapExpr{
				Keys:   []string{"name"},
				Values: []dcl.Expression{&dcl.LiteralString{Value: "b"}},
			},
		}}
		got, err := exprToValue(expr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m1 := provider.NewOrderedMap()
		m1.Set("name", provider.StringVal("a"))
		m2 := provider.NewOrderedMap()
		m2.Set("name", provider.StringVal("b"))
		want := provider.ListVal([]provider.Value{provider.MapVal(m1), provider.MapVal(m2)})
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("map_with_list_value", func(t *testing.T) {
		expr := &dcl.MapExpr{
			Keys: []string{"ports"},
			Values: []dcl.Expression{
				&dcl.ListExpr{Elements: []dcl.Expression{
					&dcl.LiteralInt{Value: 80},
					&dcl.LiteralInt{Value: 443},
				}},
			},
		}
		got, err := exprToValue(expr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m := provider.NewOrderedMap()
		m.Set("ports", provider.ListVal([]provider.Value{provider.IntVal(80), provider.IntVal(443)}))
		want := provider.MapVal(m)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("function_with_ref_arg", func(t *testing.T) {
		expr := &dcl.FunctionCall{
			Name: "secret",
			Args: []dcl.Expression{
				&dcl.Reference{Parts: []string{"db", "key"}},
			},
		}
		got, err := exprToValue(expr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := provider.FuncCallVal("secret", []provider.Value{
			provider.RefVal([]string{"db", "key"}),
		})
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("deeply_nested", func(t *testing.T) {
		// MapExpr → ListExpr → MapExpr → LiteralString
		expr := &dcl.MapExpr{
			Keys: []string{"items"},
			Values: []dcl.Expression{
				&dcl.ListExpr{Elements: []dcl.Expression{
					&dcl.MapExpr{
						Keys:   []string{"value"},
						Values: []dcl.Expression{&dcl.LiteralString{Value: "deep"}},
					},
				}},
			},
		}
		got, err := exprToValue(expr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		inner := provider.NewOrderedMap()
		inner.Set("value", provider.StringVal("deep"))
		outer := provider.NewOrderedMap()
		outer.Set("items", provider.ListVal([]provider.Value{provider.MapVal(inner)}))
		want := provider.MapVal(outer)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestExprToValue_Errors(t *testing.T) {
	t.Run("nil_expression", func(t *testing.T) {
		_, err := exprToValue(nil)
		if err == nil {
			t.Fatal("expected error for nil expression")
		}
		if !strings.Contains(err.Error(), "nil expression") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "nil expression")
		}
	})

	t.Run("nil_in_list", func(t *testing.T) {
		expr := &dcl.ListExpr{Elements: []dcl.Expression{nil}}
		_, err := exprToValue(expr)
		if err == nil {
			t.Fatal("expected error for nil in list")
		}
		msg := err.Error()
		if !strings.Contains(msg, "list element 0") {
			t.Errorf("error = %q, want it to contain %q", msg, "list element 0")
		}
		if !strings.Contains(msg, "nil expression") {
			t.Errorf("error = %q, want it to contain %q", msg, "nil expression")
		}
	})

	t.Run("nil_in_map_value", func(t *testing.T) {
		expr := &dcl.MapExpr{
			Keys:   []string{"k"},
			Values: []dcl.Expression{nil},
		}
		_, err := exprToValue(expr)
		if err == nil {
			t.Fatal("expected error for nil in map value")
		}
		msg := err.Error()
		if !strings.Contains(msg, `map key "k"`) {
			t.Errorf("error = %q, want it to contain %q", msg, `map key "k"`)
		}
		if !strings.Contains(msg, "nil expression") {
			t.Errorf("error = %q, want it to contain %q", msg, "nil expression")
		}
	})

	t.Run("nil_in_function_arg", func(t *testing.T) {
		expr := &dcl.FunctionCall{
			Name: "f",
			Args: []dcl.Expression{nil},
		}
		_, err := exprToValue(expr)
		if err == nil {
			t.Fatal("expected error for nil in function arg")
		}
		msg := err.Error()
		if !strings.Contains(msg, `function "f" arg 0`) {
			t.Errorf("error = %q, want it to contain %q", msg, `function "f" arg 0`)
		}
		if !strings.Contains(msg, "nil expression") {
			t.Errorf("error = %q, want it to contain %q", msg, "nil expression")
		}
	})
}

// --- blockToResource tests ---

func TestBlockToResource_Basic(t *testing.T) {
	t.Run("attributes_only", func(t *testing.T) {
		rng := dcl.Range{Start: dcl.Pos{Filename: "test.dcl", Line: 1, Column: 1}, End: dcl.Pos{Filename: "test.dcl", Line: 3, Column: 2}}
		block := dcl.Block{
			Type:  "index",
			Label: "logs",
			Attributes: []dcl.Attribute{
				{Key: "replicas", Value: &dcl.LiteralInt{Value: 1}},
				{Key: "shards", Value: &dcl.LiteralInt{Value: 3}},
			},
			Rng: rng,
		}

		got, err := blockToResource(block, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID.Type != "index" || got.ID.Name != "logs" {
			t.Errorf("ID = %v, want {index, logs}", got.ID)
		}
		if got.SourceRange != rng {
			t.Errorf("SourceRange = %v, want %v", got.SourceRange, rng)
		}

		wantBody := provider.NewOrderedMap()
		wantBody.Set("replicas", provider.IntVal(1))
		wantBody.Set("shards", provider.IntVal(3))
		if !got.Body.Equal(wantBody) {
			t.Errorf("Body mismatch")
		}
		// Verify insertion order.
		keys := got.Body.Keys()
		if len(keys) != 2 || keys[0] != "replicas" || keys[1] != "shards" {
			t.Errorf("keys = %v, want [replicas shards]", keys)
		}
	})

	t.Run("empty_block", func(t *testing.T) {
		block := dcl.Block{Type: "template", Label: "t1"}

		got, err := blockToResource(block, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID.Type != "template" || got.ID.Name != "t1" {
			t.Errorf("ID = %v, want {template, t1}", got.ID)
		}
		if got.Body == nil {
			t.Fatal("Body should be non-nil")
		}
		if got.Body.Len() != 0 {
			t.Errorf("Body.Len() = %d, want 0", got.Body.Len())
		}
	})

	t.Run("multiple_attribute_types", func(t *testing.T) {
		block := dcl.Block{
			Type:  "config",
			Label: "main",
			Attributes: []dcl.Attribute{
				{Key: "name", Value: &dcl.LiteralString{Value: "primary"}},
				{Key: "port", Value: &dcl.LiteralInt{Value: 9200}},
				{Key: "enabled", Value: &dcl.LiteralBool{Value: true}},
			},
		}

		got, err := blockToResource(block, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantBody := provider.NewOrderedMap()
		wantBody.Set("name", provider.StringVal("primary"))
		wantBody.Set("port", provider.IntVal(9200))
		wantBody.Set("enabled", provider.BoolVal(true))
		if !got.Body.Equal(wantBody) {
			t.Errorf("Body mismatch")
		}
	})
}

func TestBlockToResource_NestedBlocks(t *testing.T) {
	t.Run("single_unlabeled", func(t *testing.T) {
		block := dcl.Block{
			Type:  "policy",
			Label: "p",
			Blocks: []dcl.Block{
				{
					Type: "actions",
					Attributes: []dcl.Attribute{
						{Key: "delete", Value: &dcl.LiteralBool{Value: true}},
					},
				},
			},
		}

		got, err := blockToResource(block, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actionsInner := provider.NewOrderedMap()
		actionsInner.Set("delete", provider.BoolVal(true))
		wantBody := provider.NewOrderedMap()
		wantBody.Set("actions", provider.MapVal(actionsInner))
		if !got.Body.Equal(wantBody) {
			t.Errorf("Body mismatch")
		}
	})

	t.Run("single_labeled", func(t *testing.T) {
		block := dcl.Block{
			Type:  "policy",
			Label: "p",
			Blocks: []dcl.Block{
				{
					Type:  "state",
					Label: "hot",
					Attributes: []dcl.Attribute{
						{Key: "priority", Value: &dcl.LiteralInt{Value: 100}},
					},
				},
			},
		}

		got, err := blockToResource(block, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// "state" → MapVal({"hot": MapVal({priority: 100})})
		priorityMap := provider.NewOrderedMap()
		priorityMap.Set("priority", provider.IntVal(100))
		hotWrapper := provider.NewOrderedMap()
		hotWrapper.Set("hot", provider.MapVal(priorityMap))
		wantBody := provider.NewOrderedMap()
		wantBody.Set("state", provider.MapVal(hotWrapper))
		if !got.Body.Equal(wantBody) {
			t.Errorf("Body mismatch")
		}
	})

	t.Run("multiple_same_type", func(t *testing.T) {
		block := dcl.Block{
			Type:  "policy",
			Label: "lifecycle",
			Blocks: []dcl.Block{
				{
					Type:  "state",
					Label: "hot",
					Attributes: []dcl.Attribute{
						{Key: "priority", Value: &dcl.LiteralInt{Value: 100}},
					},
				},
				{
					Type:  "state",
					Label: "warm",
					Attributes: []dcl.Attribute{
						{Key: "priority", Value: &dcl.LiteralInt{Value: 50}},
					},
				},
				{
					Type:  "state",
					Label: "delete",
					Attributes: []dcl.Attribute{
						{Key: "priority", Value: &dcl.LiteralInt{Value: 0}},
					},
				},
			},
		}

		got, err := blockToResource(block, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// "state" → ListVal of 3 wrapped MapVals
		makeState := func(label string, priority int64) provider.Value {
			inner := provider.NewOrderedMap()
			inner.Set("priority", provider.IntVal(priority))
			wrapper := provider.NewOrderedMap()
			wrapper.Set(label, provider.MapVal(inner))
			return provider.MapVal(wrapper)
		}
		wantBody := provider.NewOrderedMap()
		wantBody.Set("state", provider.ListVal([]provider.Value{
			makeState("hot", 100),
			makeState("warm", 50),
			makeState("delete", 0),
		}))
		if !got.Body.Equal(wantBody) {
			t.Errorf("Body mismatch")
		}
	})

	t.Run("mixed_attrs_and_blocks", func(t *testing.T) {
		block := dcl.Block{
			Type:  "policy",
			Label: "p",
			Attributes: []dcl.Attribute{
				{Key: "description", Value: &dcl.LiteralString{Value: "test policy"}},
			},
			Blocks: []dcl.Block{
				{
					Type: "actions",
					Attributes: []dcl.Attribute{
						{Key: "delete", Value: &dcl.LiteralBool{Value: true}},
					},
				},
			},
		}

		got, err := blockToResource(block, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Attrs first in OrderedMap, then block entries.
		keys := got.Body.Keys()
		if len(keys) != 2 || keys[0] != "description" || keys[1] != "actions" {
			t.Errorf("keys = %v, want [description actions]", keys)
		}

		actionsInner := provider.NewOrderedMap()
		actionsInner.Set("delete", provider.BoolVal(true))
		wantBody := provider.NewOrderedMap()
		wantBody.Set("description", provider.StringVal("test policy"))
		wantBody.Set("actions", provider.MapVal(actionsInner))
		if !got.Body.Equal(wantBody) {
			t.Errorf("Body mismatch")
		}
	})

	t.Run("multiple_different_types", func(t *testing.T) {
		block := dcl.Block{
			Type:  "policy",
			Label: "p",
			Blocks: []dcl.Block{
				{
					Type: "actions",
					Attributes: []dcl.Attribute{
						{Key: "delete", Value: &dcl.LiteralBool{Value: true}},
					},
				},
				{
					Type: "transition",
					Attributes: []dcl.Attribute{
						{Key: "dest", Value: &dcl.LiteralString{Value: "warm"}},
					},
				},
			},
		}

		got, err := blockToResource(block, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actionsInner := provider.NewOrderedMap()
		actionsInner.Set("delete", provider.BoolVal(true))
		transitionInner := provider.NewOrderedMap()
		transitionInner.Set("dest", provider.StringVal("warm"))

		wantBody := provider.NewOrderedMap()
		wantBody.Set("actions", provider.MapVal(actionsInner))
		wantBody.Set("transition", provider.MapVal(transitionInner))
		if !got.Body.Equal(wantBody) {
			t.Errorf("Body mismatch")
		}
	})
}

func TestBlockToResource_DeepNesting(t *testing.T) {
	t.Run("three_levels", func(t *testing.T) {
		// policy → state "hot" → transition { dest = "warm" }
		block := dcl.Block{
			Type:  "policy",
			Label: "lifecycle",
			Blocks: []dcl.Block{
				{
					Type:  "state",
					Label: "hot",
					Blocks: []dcl.Block{
						{
							Type: "transition",
							Attributes: []dcl.Attribute{
								{Key: "dest", Value: &dcl.LiteralString{Value: "warm"}},
							},
						},
					},
				},
			},
		}

		got, err := blockToResource(block, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// transition inner: {dest: "warm"}
		transInner := provider.NewOrderedMap()
		transInner.Set("dest", provider.StringVal("warm"))
		// state "hot" inner: {transition: MapVal(transInner)}
		stateInner := provider.NewOrderedMap()
		stateInner.Set("transition", provider.MapVal(transInner))
		// labeled wrapper: {"hot": MapVal(stateInner)}
		hotWrapper := provider.NewOrderedMap()
		hotWrapper.Set("hot", provider.MapVal(stateInner))

		wantBody := provider.NewOrderedMap()
		wantBody.Set("state", provider.MapVal(hotWrapper))
		if !got.Body.Equal(wantBody) {
			t.Errorf("Body mismatch")
		}
	})

	t.Run("full_ism", func(t *testing.T) {
		// Full ISM policy: 3 states with transitions and actions.
		block := dcl.Block{
			Type:  "opensearch_ism_policy",
			Label: "hot_warm_delete",
			Attributes: []dcl.Attribute{
				{Key: "description", Value: &dcl.LiteralString{Value: "lifecycle policy"}},
			},
			Blocks: []dcl.Block{
				{
					Type:  "state",
					Label: "hot",
					Attributes: []dcl.Attribute{
						{Key: "priority", Value: &dcl.LiteralInt{Value: 100}},
					},
					Blocks: []dcl.Block{
						{
							Type: "transition",
							Attributes: []dcl.Attribute{
								{Key: "dest", Value: &dcl.LiteralString{Value: "warm"}},
								{Key: "min_index_age", Value: &dcl.LiteralString{Value: "7d"}},
							},
						},
					},
				},
				{
					Type:  "state",
					Label: "warm",
					Attributes: []dcl.Attribute{
						{Key: "priority", Value: &dcl.LiteralInt{Value: 50}},
					},
					Blocks: []dcl.Block{
						{
							Type: "actions",
							Attributes: []dcl.Attribute{
								{Key: "force_merge", Value: &dcl.LiteralBool{Value: true}},
							},
						},
						{
							Type: "transition",
							Attributes: []dcl.Attribute{
								{Key: "dest", Value: &dcl.LiteralString{Value: "delete"}},
								{Key: "min_index_age", Value: &dcl.LiteralString{Value: "30d"}},
							},
						},
					},
				},
				{
					Type:  "state",
					Label: "delete",
					Attributes: []dcl.Attribute{
						{Key: "priority", Value: &dcl.LiteralInt{Value: 0}},
					},
					Blocks: []dcl.Block{
						{
							Type: "actions",
							Attributes: []dcl.Attribute{
								{Key: "delete", Value: &dcl.LiteralBool{Value: true}},
							},
						},
					},
				},
			},
		}

		got, err := blockToResource(block, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.ID.Type != "opensearch_ism_policy" || got.ID.Name != "hot_warm_delete" {
			t.Errorf("ID = %v", got.ID)
		}

		// Build expected structure.
		// hot state
		hotTrans := provider.NewOrderedMap()
		hotTrans.Set("dest", provider.StringVal("warm"))
		hotTrans.Set("min_index_age", provider.StringVal("7d"))
		hotInner := provider.NewOrderedMap()
		hotInner.Set("priority", provider.IntVal(100))
		hotInner.Set("transition", provider.MapVal(hotTrans))
		hotWrapped := provider.NewOrderedMap()
		hotWrapped.Set("hot", provider.MapVal(hotInner))

		// warm state
		warmActions := provider.NewOrderedMap()
		warmActions.Set("force_merge", provider.BoolVal(true))
		warmTrans := provider.NewOrderedMap()
		warmTrans.Set("dest", provider.StringVal("delete"))
		warmTrans.Set("min_index_age", provider.StringVal("30d"))
		warmInner := provider.NewOrderedMap()
		warmInner.Set("priority", provider.IntVal(50))
		warmInner.Set("actions", provider.MapVal(warmActions))
		warmInner.Set("transition", provider.MapVal(warmTrans))
		warmWrapped := provider.NewOrderedMap()
		warmWrapped.Set("warm", provider.MapVal(warmInner))

		// delete state
		deleteActions := provider.NewOrderedMap()
		deleteActions.Set("delete", provider.BoolVal(true))
		deleteInner := provider.NewOrderedMap()
		deleteInner.Set("priority", provider.IntVal(0))
		deleteInner.Set("actions", provider.MapVal(deleteActions))
		deleteWrapped := provider.NewOrderedMap()
		deleteWrapped.Set("delete", provider.MapVal(deleteInner))

		wantBody := provider.NewOrderedMap()
		wantBody.Set("description", provider.StringVal("lifecycle policy"))
		wantBody.Set("state", provider.ListVal([]provider.Value{
			provider.MapVal(hotWrapped),
			provider.MapVal(warmWrapped),
			provider.MapVal(deleteWrapped),
		}))

		if !got.Body.Equal(wantBody) {
			t.Errorf("Body mismatch for full ISM policy")
		}
	})
}

func TestBlockToResource_Errors(t *testing.T) {
	t.Run("nil_attribute_value", func(t *testing.T) {
		block := dcl.Block{
			Type:  "index",
			Label: "x",
			Attributes: []dcl.Attribute{
				{Key: "bad", Value: nil},
			},
		}
		_, err := blockToResource(block, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		if !strings.Contains(msg, `attribute "bad"`) {
			t.Errorf("error = %q, want it to contain %q", msg, `attribute "bad"`)
		}
		if !strings.Contains(msg, "nil expression") {
			t.Errorf("error = %q, want it to contain %q", msg, "nil expression")
		}
	})

	t.Run("nil_in_nested_block", func(t *testing.T) {
		block := dcl.Block{
			Type:  "policy",
			Label: "p",
			Blocks: []dcl.Block{
				{
					Type: "actions",
					Attributes: []dcl.Attribute{
						{Key: "bad", Value: nil},
					},
				},
			},
		}
		_, err := blockToResource(block, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		if !strings.Contains(msg, `block "actions"`) {
			t.Errorf("error = %q, want it to contain %q", msg, `block "actions"`)
		}
		if !strings.Contains(msg, `attribute "bad"`) {
			t.Errorf("error = %q, want it to contain %q", msg, `attribute "bad"`)
		}
	})

	t.Run("nil_in_multi_block", func(t *testing.T) {
		block := dcl.Block{
			Type:  "policy",
			Label: "p",
			Blocks: []dcl.Block{
				{
					Type:  "state",
					Label: "ok",
					Attributes: []dcl.Attribute{
						{Key: "priority", Value: &dcl.LiteralInt{Value: 1}},
					},
				},
				{
					Type:  "state",
					Label: "bad",
					Attributes: []dcl.Attribute{
						{Key: "broken", Value: nil},
					},
				},
			},
		}
		_, err := blockToResource(block, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		if !strings.Contains(msg, `block "state"[1]`) {
			t.Errorf("error = %q, want it to contain %q", msg, `block "state"[1]`)
		}
		if !strings.Contains(msg, `attribute "broken"`) {
			t.Errorf("error = %q, want it to contain %q", msg, `attribute "broken"`)
		}
	})
}
