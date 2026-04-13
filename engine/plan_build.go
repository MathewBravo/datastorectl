package engine

import "github.com/MathewBravo/datastorectl/provider"

// BuildPlan computes a Plan by matching desired and live resources on
// ResourceID, then diffing each pair. Desired-only resources produce
// ChangeCreate, live-only produce ChangeDelete, and matched pairs produce
// ChangeUpdate or ChangeNoOp depending on whether the diff has changes.
func BuildPlan(desired, live []provider.Resource) *Plan {
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

		diff := DiffResources(*d, l)
		liveRef := &live[findIndex(live, l.ID)]
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
