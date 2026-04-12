package engine

import (
	"fmt"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// exprToValue converts a DCL AST expression into a provider Value.
// Literals become concrete values. Identifier becomes a StringVal.
// Reference and FunctionCall become placeholder values for later resolution.
func exprToValue(expr dcl.Expression) (provider.Value, error) {
	if expr == nil {
		return provider.Value{}, fmt.Errorf("cannot convert nil expression")
	}

	switch e := expr.(type) {
	case *dcl.LiteralString:
		return provider.StringVal(e.Value), nil

	case *dcl.LiteralInt:
		return provider.IntVal(e.Value), nil

	case *dcl.LiteralFloat:
		return provider.FloatVal(e.Value), nil

	case *dcl.LiteralBool:
		return provider.BoolVal(e.Value), nil

	case *dcl.Identifier:
		return provider.StringVal(e.Name), nil

	case *dcl.Reference:
		return provider.RefVal(e.Parts), nil

	case *dcl.FunctionCall:
		args := make([]provider.Value, len(e.Args))
		for i, arg := range e.Args {
			v, err := exprToValue(arg)
			if err != nil {
				return provider.Value{}, fmt.Errorf("function %q arg %d: %w", e.Name, i, err)
			}
			args[i] = v
		}
		return provider.FuncCallVal(e.Name, args), nil

	case *dcl.ListExpr:
		elems := make([]provider.Value, len(e.Elements))
		for i, elem := range e.Elements {
			v, err := exprToValue(elem)
			if err != nil {
				return provider.Value{}, fmt.Errorf("list element %d: %w", i, err)
			}
			elems[i] = v
		}
		return provider.ListVal(elems), nil

	case *dcl.MapExpr:
		m := provider.NewOrderedMap()
		for i, val := range e.Values {
			v, err := exprToValue(val)
			if err != nil {
				return provider.Value{}, fmt.Errorf("map key %q: %w", e.Keys[i], err)
			}
			m.Set(e.Keys[i], v)
		}
		return provider.MapVal(m), nil

	default:
		return provider.Value{}, fmt.Errorf("unsupported expression type %T", expr)
	}
}
