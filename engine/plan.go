package engine

import (
	"fmt"

	"github.com/MathewBravo/datastorectl/provider"
)

// ChangeType classifies what action a resource change represents.
type ChangeType int

const (
	ChangeNoOp   ChangeType = iota // resource is unchanged
	ChangeCreate                   // resource exists in desired but not live
	ChangeUpdate                   // resource exists in both but differs
	ChangeDelete                   // resource exists in live but not desired
)

func (c ChangeType) String() string {
	switch c {
	case ChangeNoOp:
		return "no-op"
	case ChangeCreate:
		return "create"
	case ChangeUpdate:
		return "update"
	case ChangeDelete:
		return "delete"
	default:
		return fmt.Sprintf("ChangeType(%d)", int(c))
	}
}

// ResourceChange describes a single planned change to a resource.
type ResourceChange struct {
	ID      provider.ResourceID
	Type    ChangeType
	Desired *provider.Resource // nil for deletes
	Live    *provider.Resource // nil for creates
	Diff    ResourceDiff
}

// Plan holds the full set of resource changes the engine intends to apply.
type Plan struct {
	Changes []ResourceChange
}

// HasChanges reports whether the plan contains any non-no-op changes.
func (p Plan) HasChanges() bool {
	for _, c := range p.Changes {
		if c.Type != ChangeNoOp {
			return true
		}
	}
	return false
}

// Creates returns all changes with type ChangeCreate.
func (p Plan) Creates() []ResourceChange {
	return p.filterByType(ChangeCreate)
}

// Updates returns all changes with type ChangeUpdate.
func (p Plan) Updates() []ResourceChange {
	return p.filterByType(ChangeUpdate)
}

// Deletes returns all changes with type ChangeDelete.
func (p Plan) Deletes() []ResourceChange {
	return p.filterByType(ChangeDelete)
}

func (p Plan) filterByType(t ChangeType) []ResourceChange {
	var out []ResourceChange
	for _, c := range p.Changes {
		if c.Type == t {
			out = append(out, c)
		}
	}
	return out
}

// Summary returns a human-readable summary of the plan.
func (p Plan) Summary() string {
	creates, updates, deletes := len(p.Creates()), len(p.Updates()), len(p.Deletes())
	return fmt.Sprintf("Plan: %d to create, %d to update, %d to delete", creates, updates, deletes)
}
