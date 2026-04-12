package engine

import (
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

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
