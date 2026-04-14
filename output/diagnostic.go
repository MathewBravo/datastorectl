package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MathewBravo/datastorectl/dcl"
)

// FormatDiagnostics renders diagnostics as human-readable text with optional color.
// Errors are red, warnings are yellow. Suggestions are indented below the message.
func FormatDiagnostics(diags dcl.Diagnostics, color bool) string {
	var blocks []string
	for _, d := range diags {
		blocks = append(blocks, formatDiagnostic(d, color))
	}
	return strings.Join(blocks, "\n\n")
}

func formatDiagnostic(d dcl.Diagnostic, color bool) string {
	var b strings.Builder

	// Severity label.
	switch d.Severity {
	case dcl.SeverityError:
		b.WriteString(red("error", color))
	case dcl.SeverityWarning:
		b.WriteString(yellow("warning", color))
	}

	// Source location.
	if d.Range.Start.Line > 0 {
		fmt.Fprintf(&b, ": %s:%d:%d", d.Range.Start.Filename, d.Range.Start.Line, d.Range.Start.Column)
	}

	// Message.
	fmt.Fprintf(&b, ": %s", d.Message)

	// Suggestion.
	if d.Suggestion != "" {
		fmt.Fprintf(&b, "\n  suggestion: %s", d.Suggestion)
	}

	return b.String()
}

// --- JSON diagnostics ---

type jsonDiagnostics struct {
	Diagnostics []jsonDiagnostic `json:"diagnostics"`
}

type jsonDiagnostic struct {
	Severity   string     `json:"severity"`
	Message    string     `json:"message"`
	Range      *jsonRange `json:"range,omitempty"`
	Suggestion string     `json:"suggestion,omitempty"`
}

type jsonRange struct {
	File  string       `json:"file"`
	Start jsonPosition `json:"start"`
}

type jsonPosition struct {
	Line   int `json:"line"`
	Column int `json:"col"`
}

// FormatDiagnosticsJSON renders diagnostics as structured JSON.
func FormatDiagnosticsJSON(diags dcl.Diagnostics) ([]byte, error) {
	jd := jsonDiagnostics{
		Diagnostics: make([]jsonDiagnostic, len(diags)),
	}

	for i, d := range diags {
		jd.Diagnostics[i] = jsonDiagnostic{
			Severity:   d.Severity.String(),
			Message:    d.Message,
			Suggestion: d.Suggestion,
		}
		if d.Range.Start.Line > 0 {
			jd.Diagnostics[i].Range = &jsonRange{
				File: d.Range.Start.Filename,
				Start: jsonPosition{
					Line:   d.Range.Start.Line,
					Column: d.Range.Start.Column,
				},
			}
		}
	}

	return json.MarshalIndent(jd, "", "  ")
}
