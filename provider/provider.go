package provider

import (
	"context"
	"fmt"

	"github.com/MathewBravo/datastorectl/dcl"
)

// Operation describes the kind of mutation to apply to a resource.
type Operation int

const (
	OpCreate Operation = iota
	OpUpdate
	OpDelete
)

func (op Operation) String() string {
	switch op {
	case OpCreate:
		return "create"
	case OpUpdate:
		return "update"
	case OpDelete:
		return "delete"
	default:
		return fmt.Sprintf("Operation(%d)", int(op))
	}
}

// Provider is the contract that every provider implementation must satisfy.
// The engine calls Configure → Discover → Normalize → Validate → Apply
// to manage infrastructure resources.
type Provider interface {
	Configure(ctx context.Context, config *OrderedMap) dcl.Diagnostics
	Discover(ctx context.Context) ([]Resource, dcl.Diagnostics)
	Normalize(ctx context.Context, r Resource) (Resource, dcl.Diagnostics)
	Validate(ctx context.Context, r Resource) dcl.Diagnostics
	Apply(ctx context.Context, op Operation, r Resource) dcl.Diagnostics
	Schemas() map[string]Schema
}

// TypeOrdering declares that resources of type Before should be
// processed before resources of type After.
type TypeOrdering struct {
	Before string
	After  string
}

// TypeOrderer is an optional interface a Provider may implement to
// declare default type-level orderings. The engine collects these
// during provider configuration and feeds them into the dependency graph.
type TypeOrderer interface {
	TypeOrderings() []TypeOrdering
}
