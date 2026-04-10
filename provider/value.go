package provider

import (
	"fmt"
	"strings"
)

// Kind classifies the type of a Value.
type Kind int

const (
	KindNull   Kind = iota // zero value — var v Value is null
	KindString             // string
	KindInt                // int64
	KindFloat              // float64
	KindBool               // bool
	KindList               // []Value
	KindMap                // *OrderedMap
	KindReference          // unresolved cross-resource reference
	KindFunctionCall       // unresolved function call (e.g. secret())
)

func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindString:
		return "string"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindBool:
		return "bool"
	case KindList:
		return "list"
	case KindMap:
		return "map"
	case KindReference:
		return "reference"
	case KindFunctionCall:
		return "function_call"
	default:
		return fmt.Sprintf("Kind(%d)", int(k))
	}
}

// Value is the universal result type for DCL expressions, provider outputs,
// and differ comparisons. It uses a tagged-union layout: Kind selects which
// field holds the payload.
type Value struct {
	Kind  Kind
	Str   string
	Int   int64
	Float float64
	Bool  bool
	List     []Value
	Map      *OrderedMap
	Ref      []string // KindReference: dotted path parts (e.g. ["db", "host"])
	FuncName string   // KindFunctionCall: function name (e.g. "secret")
	FuncArgs []Value  // KindFunctionCall: argument values
}

// NullVal returns a null Value.
func NullVal() Value {
	return Value{Kind: KindNull}
}

// StringVal returns a string Value.
func StringVal(s string) Value {
	return Value{Kind: KindString, Str: s}
}

// IntVal returns an integer Value.
func IntVal(i int64) Value {
	return Value{Kind: KindInt, Int: i}
}

// FloatVal returns a float Value.
func FloatVal(f float64) Value {
	return Value{Kind: KindFloat, Float: f}
}

// BoolVal returns a boolean Value.
func BoolVal(b bool) Value {
	return Value{Kind: KindBool, Bool: b}
}

// ListVal returns a list Value.
func ListVal(elems []Value) Value {
	return Value{Kind: KindList, List: elems}
}

// MapVal returns a map Value.
func MapVal(m *OrderedMap) Value {
	return Value{Kind: KindMap, Map: m}
}

// RefVal returns a reference Value with the given path parts.
func RefVal(parts []string) Value {
	return Value{Kind: KindReference, Ref: parts}
}

// FuncCallVal returns a function call Value with the given name and arguments.
func FuncCallVal(name string, args []Value) Value {
	return Value{Kind: KindFunctionCall, FuncName: name, FuncArgs: args}
}

// Equal reports whether v and other hold the same kind and value.
// Map equality is order-sensitive.
func (v Value) Equal(other Value) bool {
	if v.Kind != other.Kind {
		return false
	}
	switch v.Kind {
	case KindNull:
		return true
	case KindString:
		return v.Str == other.Str
	case KindInt:
		return v.Int == other.Int
	case KindFloat:
		return v.Float == other.Float
	case KindBool:
		return v.Bool == other.Bool
	case KindList:
		if len(v.List) != len(other.List) {
			return false
		}
		for i := range v.List {
			if !v.List[i].Equal(other.List[i]) {
				return false
			}
		}
		return true
	case KindMap:
		return v.Map.Equal(other.Map)
	case KindReference:
		if len(v.Ref) != len(other.Ref) {
			return false
		}
		for i := range v.Ref {
			if v.Ref[i] != other.Ref[i] {
				return false
			}
		}
		return true
	case KindFunctionCall:
		if v.FuncName != other.FuncName {
			return false
		}
		if len(v.FuncArgs) != len(other.FuncArgs) {
			return false
		}
		for i := range v.FuncArgs {
			if !v.FuncArgs[i].Equal(other.FuncArgs[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// Clone returns a deep copy of the Value.
func (v Value) Clone() Value {
	switch v.Kind {
	case KindList:
		elems := make([]Value, len(v.List))
		for i, e := range v.List {
			elems[i] = e.Clone()
		}
		return Value{Kind: KindList, List: elems}
	case KindMap:
		return Value{Kind: KindMap, Map: v.Map.Clone()}
	case KindReference:
		ref := make([]string, len(v.Ref))
		copy(ref, v.Ref)
		return Value{Kind: KindReference, Ref: ref}
	case KindFunctionCall:
		args := make([]Value, len(v.FuncArgs))
		for i, a := range v.FuncArgs {
			args[i] = a.Clone()
		}
		return Value{Kind: KindFunctionCall, FuncName: v.FuncName, FuncArgs: args}
	default:
		return v
	}
}

// String returns a human-readable representation of the Value.
func (v Value) String() string {
	switch v.Kind {
	case KindNull:
		return "null"
	case KindString:
		return fmt.Sprintf("%q", v.Str)
	case KindInt:
		return fmt.Sprintf("%d", v.Int)
	case KindFloat:
		return fmt.Sprintf("%g", v.Float)
	case KindBool:
		return fmt.Sprintf("%t", v.Bool)
	case KindList:
		s := "["
		for i, elem := range v.List {
			if i > 0 {
				s += ", "
			}
			s += elem.String()
		}
		s += "]"
		return s
	case KindMap:
		s := "{"
		for i := 0; i < v.Map.Len(); i++ {
			if i > 0 {
				s += ", "
			}
			s += v.Map.keys[i] + ": " + v.Map.values[i].String()
		}
		s += "}"
		return s
	case KindReference:
		return strings.Join(v.Ref, ".")
	case KindFunctionCall:
		s := v.FuncName + "("
		for i, arg := range v.FuncArgs {
			if i > 0 {
				s += ", "
			}
			s += arg.String()
		}
		s += ")"
		return s
	default:
		return fmt.Sprintf("<unknown kind %d>", int(v.Kind))
	}
}
