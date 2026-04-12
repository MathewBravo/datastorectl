package engine

import (
	"fmt"

	"github.com/MathewBravo/datastorectl/provider"
)

// ExtractReferences walks a resource body and returns the ResourceID
// of every cross-resource reference it contains.
func ExtractReferences(r provider.Resource) []provider.ResourceID {
	out := []provider.ResourceID{}
	if r.Body == nil {
		return out
	}
	for _, key := range r.Body.Keys() {
		v, _ := r.Body.Get(key)
		collectRefs(v, &out)
	}
	return out
}

// ResolveReferences walks a resource body and replaces KindReference values
// with StringVal(target.ID.Name) looked up from the index. The original
// resource is not mutated.
func ResolveReferences(r provider.Resource, index map[provider.ResourceID]provider.Resource) (provider.Resource, error) {
	if r.Body == nil {
		return provider.Resource{ID: r.ID, Body: nil, SourceRange: r.SourceRange}, nil
	}
	resolved := r.Body.Clone()
	for _, key := range resolved.Keys() {
		v, _ := resolved.Get(key)
		rv, err := resolveRef(v, index)
		if err != nil {
			return provider.Resource{}, fmt.Errorf("attribute %q: %w", key, err)
		}
		resolved.Set(key, rv)
	}
	return provider.Resource{ID: r.ID, Body: resolved, SourceRange: r.SourceRange}, nil
}

// resolveRef recursively walks a Value tree, resolving any references.
func resolveRef(v provider.Value, index map[provider.ResourceID]provider.Resource) (provider.Value, error) {
	switch v.Kind {
	case provider.KindReference:
		if len(v.Ref) < 2 {
			return provider.Value{}, fmt.Errorf("reference has %d parts, need at least 2", len(v.Ref))
		}
		id := provider.ResourceID{Type: v.Ref[0], Name: v.Ref[1]}
		target, ok := index[id]
		if !ok {
			return provider.Value{}, fmt.Errorf("target not found: %s", id)
		}
		return provider.StringVal(target.ID.Name), nil
	case provider.KindList:
		elems := make([]provider.Value, len(v.List))
		for i, elem := range v.List {
			rv, err := resolveRef(elem, index)
			if err != nil {
				return provider.Value{}, fmt.Errorf("list element %d: %w", i, err)
			}
			elems[i] = rv
		}
		return provider.ListVal(elems), nil
	case provider.KindMap:
		m := provider.NewOrderedMap()
		for _, key := range v.Map.Keys() {
			val, _ := v.Map.Get(key)
			rv, err := resolveRef(val, index)
			if err != nil {
				return provider.Value{}, fmt.Errorf("key %q: %w", key, err)
			}
			m.Set(key, rv)
		}
		return provider.MapVal(m), nil
	case provider.KindFunctionCall:
		args := make([]provider.Value, len(v.FuncArgs))
		for i, arg := range v.FuncArgs {
			rv, err := resolveRef(arg, index)
			if err != nil {
				return provider.Value{}, fmt.Errorf("function %q argument %d: %w", v.FuncName, i, err)
			}
			args[i] = rv
		}
		return provider.FuncCallVal(v.FuncName, args), nil
	default:
		return v, nil
	}
}

// collectRefs recursively walks a Value tree, appending any
// KindReference targets to out.
func collectRefs(v provider.Value, out *[]provider.ResourceID) {
	switch v.Kind {
	case provider.KindReference:
		if len(v.Ref) >= 2 {
			*out = append(*out, provider.ResourceID{
				Type: v.Ref[0],
				Name: v.Ref[1],
			})
		}
	case provider.KindList:
		for _, elem := range v.List {
			collectRefs(elem, out)
		}
	case provider.KindMap:
		for _, key := range v.Map.Keys() {
			val, _ := v.Map.Get(key)
			collectRefs(val, out)
		}
	case provider.KindFunctionCall:
		for _, arg := range v.FuncArgs {
			collectRefs(arg, out)
		}
	}
}
