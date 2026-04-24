package engine

import (
	"context"

	"github.com/MathewBravo/datastorectl/provider"
)

// BuildPlan computes a Plan by matching desired and live resources on
// ResourceID, then diffing each pair. Desired-only resources produce
// ChangeCreate, live-only produce ChangeDelete, and matched pairs
// produce ChangeUpdate or ChangeNoOp depending on whether the diff
// has changes.
//
// When a provider implements provider.ResourceDiffer, its Equal method
// is consulted first for matched pairs. A true result collapses the
// pair to ChangeNoOp; a false result falls through to the structural
// DiffResources so attribute-level output still reaches the plan
// renderer for the ChangeUpdate case.
func BuildPlan(ctx context.Context, desired, live []provider.Resource, providers map[string]provider.Provider) *Plan {
	liveIndex := make(map[provider.ResourceID]provider.Resource, len(live))
	for _, r := range live {
		liveIndex[r.ID] = r
	}

	seen := make(map[provider.ResourceID]bool, len(desired))
	var changes []ResourceChange

	// Walk desired resources: creates and updates/no-ops.
	for i := range desired {
		d := &desired[i]
		seen[d.ID] = true

		l, found := liveIndex[d.ID]
		if !found {
			changes = append(changes, ResourceChange{
				ID:      d.ID,
				Type:    ChangeCreate,
				Desired: d,
			})
			continue
		}

		liveRef := &live[findIndex(live, l.ID)]

		// Consult the provider's ResourceDiffer if it has one. A true
		// result is enough to collapse the pair to ChangeNoOp without
		// running the structural diff. False falls through so the
		// attribute-level diff still renders.
		if p, ok := providers[d.ID.Type]; ok {
			if differ, ok := p.(provider.ResourceDiffer); ok {
				equal, diags := differ.Equal(ctx, *d, l)
				if !diags.HasErrors() && equal {
					changes = append(changes, ResourceChange{
						ID:      d.ID,
						Type:    ChangeNoOp,
						Desired: d,
						Live:    liveRef,
					})
					continue
				}
			}
		}

		diff := DiffResources(*d, l)
		if diff.HasChanges() {
			changes = append(changes, ResourceChange{
				ID:      d.ID,
				Type:    ChangeUpdate,
				Desired: d,
				Live:    liveRef,
				Diff:    diff,
			})
		} else {
			changes = append(changes, ResourceChange{
				ID:      d.ID,
				Type:    ChangeNoOp,
				Desired: d,
				Live:    liveRef,
				Diff:    diff,
			})
		}
	}

	// Walk live resources: anything not in desired is a delete.
	for i := range live {
		l := &live[i]
		if seen[l.ID] {
			continue
		}
		changes = append(changes, ResourceChange{
			ID:   l.ID,
			Type: ChangeDelete,
			Live: l,
		})
	}

	return &Plan{Changes: changes}
}

// findIndex returns the index of the resource with the given ID in the slice.
func findIndex(rs []provider.Resource, id provider.ResourceID) int {
	for i := range rs {
		if rs[i].ID == id {
			return i
		}
	}
	return -1
}
