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
// blocks produce the same ResourceID. The schemas map may be nil for
// untyped conversion (count-based fallback).
func ConvertFile(file *dcl.File, schemas map[string]provider.Schema) (*ResourceSet, error) {
	if file == nil {
		return nil, fmt.Errorf("cannot convert nil file")
	}
	if file.Diagnostics.HasErrors() {
		return nil, fmt.Errorf("file has parse errors: %s", file.Diagnostics.Error())
	}
	return ConvertBlocks(file.Blocks, schemas)
}

// ConvertBlocks converts a slice of DCL blocks into a ResourceSet.
// Unlike ConvertFile, it operates on pre-filtered blocks (e.g., after
// context blocks have been separated out by config.SplitFile).
// The schemas map may be nil for untyped conversion (count-based fallback).
func ConvertBlocks(blocks []dcl.Block, schemas map[string]provider.Schema) (*ResourceSet, error) {
	resources := make([]provider.Resource, 0, len(blocks))
	seen := map[provider.ResourceID]struct{}{}

	for i, block := range blocks {
		var schema *provider.Schema
		if schemas != nil {
			if s, ok := schemas[block.Type]; ok {
				schema = &s
			}
		}
		r, err := blockToResource(block, schema)
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
// The schema is used to determine list vs map for nested blocks; it may be nil.
func blockToResource(block dcl.Block, schema *provider.Schema) (provider.Resource, error) {
	body, err := convertBody(block.Attributes, block.Blocks, schema)
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
// Attributes are converted via exprToValue. Nested blocks are grouped by type.
// When a schema is provided, its FieldHints determine list vs map. When schema
// is nil (e.g. inside nested blocks), count-based fallback applies: single
// occurrence → MapVal, multiple → ListVal.
func convertBody(attrs []dcl.Attribute, blocks []dcl.Block, schema *provider.Schema) (*provider.OrderedMap, error) {
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

	// Convert each group. Schema hints take precedence; count-based fallback
	// is used when no schema is present.
	for _, g := range groups {
		hint := fieldHintFor(schema, g.typ)

		switch hint {
		case provider.FieldBlockList:
			// Always produce a list, even for a single block.
			elems := make([]provider.Value, len(g.blocks))
			for i, b := range g.blocks {
				nested, err := convertNestedBlock(b)
				if err != nil {
					return nil, fmt.Errorf("block %q[%d]: %w", g.typ, i, err)
				}
				elems[i] = provider.MapVal(nested)
			}
			m.Set(g.typ, provider.ListVal(elems))

		case provider.FieldBlockMap:
			// Always produce a map; error if more than one block.
			if len(g.blocks) > 1 {
				return nil, fmt.Errorf("block %q: schema declares map but %d blocks found", g.typ, len(g.blocks))
			}
			nested, err := convertNestedBlock(g.blocks[0])
			if err != nil {
				return nil, fmt.Errorf("block %q: %w", g.typ, err)
			}
			m.Set(g.typ, provider.MapVal(nested))

		default:
			// No schema hint — count-based fallback.
			if schema != nil {
				return nil, fmt.Errorf("block %q: not declared in schema for this resource type", g.typ)
			}
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
	}

	return m, nil
}

// fieldHintFor returns the FieldHint for a block type from the schema.
// Returns 0 (no hint) if the schema is nil or doesn't contain the field.
func fieldHintFor(schema *provider.Schema, blockType string) provider.FieldHint {
	if schema == nil || schema.Fields == nil {
		return 0
	}
	return schema.Fields[blockType]
}

// convertNestedBlock converts a single nested block's content into an OrderedMap.
// If the block has a label, the result is wrapped: label becomes the key.
// Nested blocks always use count-based fallback (nil schema).
func convertNestedBlock(block dcl.Block) (*provider.OrderedMap, error) {
	inner, err := convertBody(block.Attributes, block.Blocks, nil)
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
