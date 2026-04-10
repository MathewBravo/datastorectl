package provider

import "fmt"

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
	List  []Value
	Map   *OrderedMap
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
		vLen := v.Map.Len()
		oLen := other.Map.Len()
		if vLen != oLen {
			return false
		}
		for i := 0; i < vLen; i++ {
			if v.Map.keys[i] != other.Map.keys[i] {
				return false
			}
			if !v.Map.values[i].Equal(other.Map.values[i]) {
				return false
			}
		}
		return true
	default:
		return false
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
	default:
		return fmt.Sprintf("<unknown kind %d>", int(v.Kind))
	}
}
