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

	// Both non-null, not equal → modified (scalar fallback).
	return []ValueDiff{{Kind: DiffModified, Path: path, Old: old, New: new}}
}
