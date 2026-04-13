package engine

import (
	"fmt"

	"github.com/MathewBravo/datastorectl/provider"
)

// ResultStatus classifies the outcome of applying a resource change.
type ResultStatus int

const (
	StatusSuccess ResultStatus = iota
	StatusFailed
	StatusSkipped
)

func (s ResultStatus) String() string {
	switch s {
	case StatusSuccess:
		return "success"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	default:
		return fmt.Sprintf("ResultStatus(%d)", int(s))
	}
}

// ResourceResult records the outcome of applying a single resource change.
type ResourceResult struct {
	ID         provider.ResourceID
	Status     ResultStatus
	Error      error
	ChangeType ChangeType
}

// ApplyResult aggregates the results of applying an entire plan.
type ApplyResult struct {
	Results []ResourceResult
}

// HasErrors reports whether any resource failed during apply.
func (r *ApplyResult) HasErrors() bool {
	for _, res := range r.Results {
		if res.Status == StatusFailed {
			return true
		}
	}
	return false
}

// Failed returns all results with StatusFailed.
func (r *ApplyResult) Failed() []ResourceResult {
	return r.filterByStatus(StatusFailed)
}

// Skipped returns all results with StatusSkipped.
func (r *ApplyResult) Skipped() []ResourceResult {
	return r.filterByStatus(StatusSkipped)
}

// Summary returns a human-readable summary of apply outcomes.
func (r *ApplyResult) Summary() string {
	var succeeded, failed, skipped int
	for _, res := range r.Results {
		switch res.Status {
		case StatusSuccess:
			succeeded++
		case StatusFailed:
			failed++
		case StatusSkipped:
			skipped++
		}
	}
	return fmt.Sprintf("Apply complete: %d succeeded, %d failed, %d skipped", succeeded, failed, skipped)
}

func (r *ApplyResult) filterByStatus(s ResultStatus) []ResourceResult {
	var out []ResourceResult
	for _, res := range r.Results {
		if res.Status == s {
			out = append(out, res)
		}
	}
	return out
}
