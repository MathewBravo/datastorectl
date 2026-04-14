package engine

import (
	"fmt"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// ResourceSet is a flat, validated collection of resources produced by
// converting all blocks in a DCL file.
type ResourceSet struct {
	Resources []provider.Resource
}

// ConvertFile converts all blocks in a parsed DCL file into a ResourceSet.
// It returns an error if the file contains parse errors or if any two
// blocks produce the same ResourceID.
func ConvertFile(file *dcl.File) (*ResourceSet, error) {
	if file == nil {
		return nil, fmt.Errorf("cannot convert nil file")
	}
	if file.Diagnostics.HasErrors() {
		return nil, fmt.Errorf("file has parse errors: %s", file.Diagnostics.Error())
	}
	return ConvertBlocks(file.Blocks)
}

// ConvertBlocks converts a slice of DCL blocks into a ResourceSet.
// Unlike ConvertFile, it operates on pre-filtered blocks (e.g., after
// context blocks have been separated out by config.SplitFile).
func ConvertBlocks(blocks []dcl.Block) (*ResourceSet, error) {
	resources := make([]provider.Resource, 0, len(blocks))
	seen := map[provider.ResourceID]struct{}{}

	for i, block := range blocks {
		r, err := blockToResource(block)
		if err != nil {
			return nil, fmt.Errorf("block %d (%s %q): %w", i, block.Type, block.Label, err)
		}
		if _, dup := seen[r.ID]; dup {
			return nil, fmt.Errorf("duplicate resource %q", r.ID)
		}
		seen[r.ID] = struct{}{}
		resources = append(resources, r)
	}

	return &ResourceSet{Resources: resources}, nil
}

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

// blockToResource converts a top-level DCL Block into a provider Resource.
// Block.Type → ResourceID.Type, Block.Label → ResourceID.Name.
func blockToResource(block dcl.Block) (provider.Resource, error) {
	body, err := convertBody(block.Attributes, block.Blocks)
	if err != nil {
		return provider.Resource{}, err
	}
	return provider.Resource{
		ID:          provider.ResourceID{Type: block.Type, Name: block.Label},
		Body:        body,
		SourceRange: block.Rng,
	}, nil
}

// convertBody converts a block's attributes and nested blocks into an OrderedMap.
// Attributes are converted via exprToValue. Nested blocks are grouped by type:
// single-occurrence types become a MapVal, multi-occurrence become a ListVal.
func convertBody(attrs []dcl.Attribute, blocks []dcl.Block) (*provider.OrderedMap, error) {
	m := provider.NewOrderedMap()

	// Attributes first.
	for _, attr := range attrs {
		v, err := exprToValue(attr.Value)
		if err != nil {
			return nil, fmt.Errorf("attribute %q: %w", attr.Key, err)
		}
		m.Set(attr.Key, v)
	}

	// Group nested blocks by type, preserving first-occurrence order.
	type blockGroup struct {
		typ    string
		blocks []dcl.Block
	}
	var groups []blockGroup
	idx := map[string]int{} // type → index into groups
	for _, b := range blocks {
		if i, ok := idx[b.Type]; ok {
			groups[i].blocks = append(groups[i].blocks, b)
		} else {
			idx[b.Type] = len(groups)
			groups = append(groups, blockGroup{typ: b.Type, blocks: []dcl.Block{b}})
		}
	}

	// Convert each group.
	for _, g := range groups {
		if len(g.blocks) == 1 {
			nested, err := convertNestedBlock(g.blocks[0])
			if err != nil {
				return nil, fmt.Errorf("block %q: %w", g.typ, err)
			}
			m.Set(g.typ, provider.MapVal(nested))
		} else {
			elems := make([]provider.Value, len(g.blocks))
			for i, b := range g.blocks {
				nested, err := convertNestedBlock(b)
				if err != nil {
					return nil, fmt.Errorf("block %q[%d]: %w", g.typ, i, err)
				}
				elems[i] = provider.MapVal(nested)
			}
			m.Set(g.typ, provider.ListVal(elems))
		}
	}

	return m, nil
}

// convertNestedBlock converts a single nested block's content into an OrderedMap.
// If the block has a label, the result is wrapped: label becomes the key.
func convertNestedBlock(block dcl.Block) (*provider.OrderedMap, error) {
	inner, err := convertBody(block.Attributes, block.Blocks)
	if err != nil {
		return nil, err
	}
	if block.Label != "" {
		outer := provider.NewOrderedMap()
		outer.Set(block.Label, provider.MapVal(inner))
		return outer, nil
	}
	return inner, nil
}
