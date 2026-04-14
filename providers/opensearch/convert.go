package opensearch

import (
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/MathewBravo/datastorectl/provider"
)

// jsonToValue converts an encoding/json unmarshaled any to a provider.Value.
// JSON maps are sorted alphabetically by key to produce deterministic OrderedMaps.
func jsonToValue(v any) provider.Value {
	switch val := v.(type) {
	case nil:
		return provider.NullVal()
	case string:
		return provider.StringVal(val)
	case bool:
		return provider.BoolVal(val)
	case float64:
		if val == math.Trunc(val) && !math.IsInf(val, 0) && !math.IsNaN(val) {
			return provider.IntVal(int64(val))
		}
		return provider.FloatVal(val)
	case []any:
		elems := make([]provider.Value, len(val))
		for i, e := range val {
			elems[i] = jsonToValue(e)
		}
		return provider.ListVal(elems)
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		m := provider.NewOrderedMap()
		for _, k := range keys {
			m.Set(k, jsonToValue(val[k]))
		}
		return provider.MapVal(m)
	default:
		return provider.NullVal()
	}
}

// valueToJSON converts a provider.Value back to a Go value suitable for json.Marshal.
func valueToJSON(v provider.Value) any {
	switch v.Kind {
	case provider.KindNull:
		return nil
	case provider.KindString:
		return v.Str
	case provider.KindInt:
		return v.Int
	case provider.KindFloat:
		return v.Float
	case provider.KindBool:
		return v.Bool
	case provider.KindList:
		out := make([]any, len(v.List))
		for i, e := range v.List {
			out[i] = valueToJSON(e)
		}
		return out
	case provider.KindMap:
		out := make(map[string]any, v.Map.Len())
		keys := v.Map.Keys()
		for _, k := range keys {
			val, _ := v.Map.Get(k)
			out[k] = valueToJSON(val)
		}
		return out
	default:
		return nil
	}
}

// sortStringList returns a sorted copy of a KindList of KindString values.
func sortStringList(v provider.Value) provider.Value {
	if v.Kind != provider.KindList || len(v.List) == 0 {
		return v
	}
	sorted := make([]provider.Value, len(v.List))
	copy(sorted, v.List)
	slices.SortFunc(sorted, func(a, b provider.Value) int {
		if a.Str < b.Str {
			return -1
		}
		if a.Str > b.Str {
			return 1
		}
		return 0
	})
	return provider.ListVal(sorted)
}

// isTruthy checks if a key in a raw JSON map holds a boolean true.
func isTruthy(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// stripEmptyListField deletes a key from a raw JSON map if its value is an empty slice.
func stripEmptyListField(m map[string]any, key string) {
	v, ok := m[key]
	if !ok {
		return
	}
	if list, ok := v.([]any); ok && len(list) == 0 {
		delete(m, key)
	}
}

// stripEmptyStringField deletes a key from a raw JSON map if its value is an empty string.
func stripEmptyStringField(m map[string]any, key string) {
	v, ok := m[key]
	if !ok {
		return
	}
	if s, ok := v.(string); ok && s == "" {
		delete(m, key)
	}
}

// stripEmptyValueList deletes a key from an OrderedMap if its value is an empty list.
func stripEmptyValueList(m *provider.OrderedMap, key string) {
	v, ok := m.Get(key)
	if !ok {
		return
	}
	if v.Kind == provider.KindList && len(v.List) == 0 {
		m.Delete(key)
	}
}

// stripEmptyValueString deletes a key from an OrderedMap if its value is an empty string.
func stripEmptyValueString(m *provider.OrderedMap, key string) {
	v, ok := m.Get(key)
	if !ok {
		return
	}
	if v.Kind == provider.KindString && v.Str == "" {
		m.Delete(key)
	}
}

// stripEmptyMapField deletes a key from a raw JSON map if its value is an empty map.
func stripEmptyMapField(m map[string]any, key string) {
	v, ok := m[key]
	if !ok {
		return
	}
	if obj, ok := v.(map[string]any); ok && len(obj) == 0 {
		delete(m, key)
	}
}

// stripEmptyValueMap deletes a key from an OrderedMap if its value is an empty map.
func stripEmptyValueMap(m *provider.OrderedMap, key string) {
	v, ok := m.Get(key)
	if !ok {
		return
	}
	if v.Kind == provider.KindMap && v.Map.Len() == 0 {
		m.Delete(key)
	}
}

// requireStringList validates that a key in an OrderedMap is a present list of strings.
func requireStringList(prefix string, m *provider.OrderedMap, key string) error {
	v, ok := m.Get(key)
	if !ok {
		return fmt.Errorf("%s.%s is required", prefix, key)
	}
	if v.Kind != provider.KindList {
		return fmt.Errorf("%s.%s must be a list, got %s", prefix, key, v.Kind)
	}
	for i, elem := range v.List {
		if elem.Kind != provider.KindString {
			return fmt.Errorf("%s.%s[%d] must be a string, got %s", prefix, key, i, elem.Kind)
		}
	}
	return nil
}

// optionalStringList validates that a key, if present, is a list of strings.
func optionalStringList(prefix string, m *provider.OrderedMap, key string) error {
	v, ok := m.Get(key)
	if !ok {
		return nil
	}
	if v.Kind != provider.KindList {
		return fmt.Errorf("%s.%s must be a list, got %s", prefix, key, v.Kind)
	}
	for i, elem := range v.List {
		if elem.Kind != provider.KindString {
			return fmt.Errorf("%s.%s[%d] must be a string, got %s", prefix, key, i, elem.Kind)
		}
	}
	return nil
}
