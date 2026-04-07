// diagnostic.go defines types for reporting errors and warnings with source locations.
package dcl

import (
	"fmt"
	"strings"
)

// Severity indicates the severity of a diagnostic.
type Severity int

const (
	SeverityError   Severity = iota
	SeverityWarning
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// Diagnostic represents a single diagnostic message tied to a source range.
type Diagnostic struct {
	Severity   Severity
	Message    string
	Range      Range
	Suggestion string
}

func (d Diagnostic) String() string {
	s := fmt.Sprintf("%s: %s: %s", d.Range.Start, d.Severity, d.Message)
	if d.Suggestion != "" {
		s += fmt.Sprintf(" (%s)", d.Suggestion)
	}
	return s
}

// Diagnostics is a slice of Diagnostic values.
type Diagnostics []Diagnostic

// HasErrors returns true if any diagnostic has error severity.
func (ds Diagnostics) HasErrors() bool {
	for _, d := range ds {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Error returns a combined string of all diagnostics, or empty if none.
func (ds Diagnostics) Error() string {
	if len(ds) == 0 {
		return ""
	}
	var b strings.Builder
	for i, d := range ds {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(d.String())
	}
	return b.String()
}

// Append merges another Diagnostics slice into this one.
func (ds *Diagnostics) Append(other Diagnostics) {
	*ds = append(*ds, other...)
}
