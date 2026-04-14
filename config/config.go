// Package config extracts and validates connection contexts from parsed DCL files.
// Contexts define how to connect to a provider (endpoint, auth, credentials).
// This package bridges DCL AST blocks and the provider.OrderedMap configs
// that engine.ConfigureProviders expects.
package config

import (
	"fmt"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// Context is a validated, structured representation of a DCL context block.
type Context struct {
	Name     string              // block label (e.g., "prod-opensearch")
	Provider string              // value of the "provider" attribute
	Attrs    *provider.OrderedMap // remaining attributes (endpoint, auth, etc.)
}

// SplitFile separates context blocks from resource blocks in a parsed DCL file.
// Context blocks have Type == "context". Everything else is treated as a resource block.
func SplitFile(file *dcl.File) (contexts []dcl.Block, resources []dcl.Block) {
	for _, block := range file.Blocks {
		if block.Type == "context" {
			contexts = append(contexts, block)
		} else {
			resources = append(resources, block)
		}
	}
	return contexts, resources
}

// ParseContexts validates and converts raw context blocks into structured Contexts.
//
// Each context block must have:
//   - A label (the context name)
//   - A "provider" attribute that is a string or identifier
//
// The provider attribute is extracted and stored separately; it does not appear in Attrs.
// Duplicate context names produce an error.
func ParseContexts(blocks []dcl.Block) ([]Context, error) {
	seen := map[string]struct{}{}
	contexts := make([]Context, 0, len(blocks))

	for _, block := range blocks {
		if block.Label == "" {
			return nil, fmt.Errorf("context block is missing a name — use: context \"my-name\" { ... }")
		}

		if _, dup := seen[block.Label]; dup {
			return nil, fmt.Errorf("duplicate context name %q — each context must have a unique name", block.Label)
		}
		seen[block.Label] = struct{}{}

		providerName, attrs, err := extractContextAttrs(block)
		if err != nil {
			return nil, fmt.Errorf("context %q: %s", block.Label, err)
		}

		contexts = append(contexts, Context{
			Name:     block.Label,
			Provider: providerName,
			Attrs:    attrs,
		})
	}

	return contexts, nil
}

// extractContextAttrs pulls the "provider" attribute out of a context block
// and converts the remaining attributes into a provider.OrderedMap.
func extractContextAttrs(block dcl.Block) (string, *provider.OrderedMap, error) {
	var providerName string
	attrs := provider.NewOrderedMap()

	for _, attr := range block.Attributes {
		if attr.Key == "provider" {
			name, err := identifierString(attr.Value)
			if err != nil {
				return "", nil, fmt.Errorf("\"provider\" must be a name (e.g. opensearch), got %T", attr.Value)
			}
			providerName = name
			continue
		}

		v, err := exprToValue(attr.Value)
		if err != nil {
			return "", nil, fmt.Errorf("attribute %q: %s", attr.Key, err)
		}
		attrs.Set(attr.Key, v)
	}

	if providerName == "" {
		return "", nil, fmt.Errorf("\"provider\" attribute is required — specify which provider this context configures (e.g. provider = opensearch)")
	}

	return providerName, attrs, nil
}

// identifierString extracts a string from an Identifier or LiteralString expression.
func identifierString(expr dcl.Expression) (string, error) {
	switch e := expr.(type) {
	case *dcl.Identifier:
		return e.Name, nil
	case *dcl.LiteralString:
		return e.Value, nil
	default:
		return "", fmt.Errorf("expected identifier or string, got %T", expr)
	}
}

// exprToValue converts a DCL AST expression into a provider.Value.
// Same logic as engine/convert.go's exprToValue — duplicated here to avoid
// an import cycle (engine will import config in a later ticket).
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
