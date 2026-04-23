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
// Changes are the actions that will execute (creates, updates, and — when
// pruning is enabled — deletes). Unmanaged carries delete changes that were
// discovered but suppressed because pruning is off; they are informational
// only and never executed.
type Plan struct {
	Changes   []ResourceChange
	Unmanaged []ResourceChange
}

// PlanOptions tunes the planning pipeline. Zero value means additive-only:
// creates and updates are planned, deletes are surfaced as Unmanaged.
type PlanOptions struct {
	// Prune includes delete changes in Plan.Changes. When false, deletes
	// move to Plan.Unmanaged and are neither displayed per-resource nor
	// executed.
	Prune bool
}

// HasChanges reports whether the plan contains any non-no-op changes.
// Unmanaged resources do not count — they are suppressed from execution.
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

// Deletes returns delete changes from Plan.Changes. This is only populated
// when PlanOptions.Prune was true; otherwise deletes live in Plan.Unmanaged.
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
// Plan.Changes containing delete entries and Plan.Unmanaged being non-empty are mutually exclusive — the engine maintains this invariant.
// Format depends on whether pruning is active (indicated by the presence of
// delete changes in Plan.Changes vs Plan.Unmanaged):
//   - additive mode, no unmanaged: "Plan: X to create, Y to update"
//   - additive mode with unmanaged: "Plan: X to create, Y to update (N unmanaged resources — use --prune to delete)"
//   - prune mode: "Plan: X to create, Y to update, Z to delete"
func (p Plan) Summary() string {
	creates, updates, deletes := len(p.Creates()), len(p.Updates()), len(p.Deletes())
	unmanaged := len(p.Unmanaged)

	if deletes > 0 {
		// Prune mode — deletes already in Changes.
		return fmt.Sprintf("Plan: %d to create, %d to update, %d to delete", creates, updates, deletes)
	}
	if unmanaged > 0 {
		return fmt.Sprintf("Plan: %d to create, %d to update (%d unmanaged resources — use --prune to delete)", creates, updates, unmanaged)
	}
	return fmt.Sprintf("Plan: %d to create, %d to update", creates, updates)
}
