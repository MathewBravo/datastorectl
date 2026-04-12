package engine

import "github.com/MathewBravo/datastorectl/provider"

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
