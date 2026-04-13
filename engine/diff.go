package engine

import (
	"fmt"

	"github.com/MathewBravo/datastorectl/provider"
)

// DiffKind classifies the type of change between two values.
type DiffKind int

const (
	DiffNone     DiffKind = iota // zero value, never appears in results
	DiffAdded                    // value exists in new but not old
	DiffRemoved                  // value exists in old but not new
	DiffModified                 // value changed between old and new
)

func (k DiffKind) String() string {
	switch k {
	case DiffNone:
		return "none"
	case DiffAdded:
		return "added"
	case DiffRemoved:
		return "removed"
	case DiffModified:
		return "modified"
	default:
		return fmt.Sprintf("DiffKind(%d)", int(k))
	}
}

// ValueDiff describes a single leaf-level change between two values.
type ValueDiff struct {
	Kind     DiffKind
	Path     string         // dot-notation path to the changed value
	Old, New provider.Value // previous and current values
}

// DiffValues compares old and new and returns a list of leaf-level diffs.
// For scalar values this produces at most one entry. The returned slice is
// designed to grow when map (#42) and list (#43) structural diffing is added.
func DiffValues(path string, old, new provider.Value) []ValueDiff {
	if old.Equal(new) {
		return nil
	}

	// One side null → added or removed.
	if old.Kind == provider.KindNull {
		return []ValueDiff{{Kind: DiffAdded, Path: path, New: new}}
	}
	if new.Kind == provider.KindNull {
		return []ValueDiff{{Kind: DiffRemoved, Path: path, Old: old}}
	}

	// Structural map diffing.
	if old.Kind == provider.KindMap && new.Kind == provider.KindMap {
		return diffMaps(path, old.Map, new.Map)
	}

	// Structural list diffing.
	if old.Kind == provider.KindList && new.Kind == provider.KindList {
		return diffLists(path, old.List, new.List)
	}

	// Both non-null, not equal → modified (scalar fallback).
	return []ValueDiff{{Kind: DiffModified, Path: path, Old: old, New: new}}
}

// diffMaps produces per-key leaf-level diffs between two ordered maps.
// Diffs are emitted in old-map key order (removals and modifications),
// then new-map key order (additions).
func diffMaps(path string, old, new *provider.OrderedMap) []ValueDiff {
	var diffs []ValueDiff

	// Walk old keys: detect removals and recurse into modifications.
	for _, key := range old.Keys() {
		childPath := path + "." + key
		if path == "" {
			childPath = key
		}
		oldVal, _ := old.Get(key)
		newVal, found := new.Get(key)
		if !found {
			diffs = append(diffs, ValueDiff{Kind: DiffRemoved, Path: childPath, Old: oldVal})
			continue
		}
		diffs = append(diffs, DiffValues(childPath, oldVal, newVal)...)
	}

	// Walk new keys: detect additions (keys already in old were handled above).
	for _, key := range new.Keys() {
		if _, found := old.Get(key); found {
			continue
		}
		childPath := path + "." + key
		if path == "" {
			childPath = key
		}
		newVal, _ := new.Get(key)
		diffs = append(diffs, ValueDiff{Kind: DiffAdded, Path: childPath, New: newVal})
	}

	return diffs
}

// diffLists produces per-element leaf-level diffs between two lists using
// positional comparison. Overlapping indices are recursed; trailing elements
// in either list are reported as additions or removals.
func diffLists(path string, old, new []provider.Value) []ValueDiff {
	var diffs []ValueDiff
	minLen := min(len(old), len(new))

	for i := 0; i < minLen; i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		diffs = append(diffs, DiffValues(childPath, old[i], new[i])...)
	}
	for i := minLen; i < len(new); i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		diffs = append(diffs, ValueDiff{Kind: DiffAdded, Path: childPath, New: new[i]})
	}
	for i := minLen; i < len(old); i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		diffs = append(diffs, ValueDiff{Kind: DiffRemoved, Path: childPath, Old: old[i]})
	}
	return diffs
}

// ResourceDiff holds the result of comparing two resources.
type ResourceDiff struct {
	ID    provider.ResourceID
	Diffs []ValueDiff
}

// HasChanges reports whether the diff contains any changes.
func (d ResourceDiff) HasChanges() bool {
	return len(d.Diffs) > 0
}

// DiffResources compares desired and live resources by diffing their bodies.
func DiffResources(desired, live provider.Resource) ResourceDiff {
	return ResourceDiff{
		ID:    desired.ID,
		Diffs: DiffValues("", provider.MapVal(live.Body), provider.MapVal(desired.Body)),
	}
}
