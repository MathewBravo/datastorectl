package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
)

func TestFormatDiagnostics_error_colored(t *testing.T) {
	diags := dcl.Diagnostics{{
		Severity: dcl.SeverityError,
		Message:  "something broke",
	}}

	got := FormatDiagnostics(diags, true)

	if !strings.Contains(got, ansiRed+"error"+ansiReset) {
		t.Errorf("expected red 'error' label, got:\n%s", got)
	}
	if !strings.Contains(got, "something broke") {
		t.Error("expected message in output")
	}
}

func TestFormatDiagnostics_warning_colored(t *testing.T) {
	diags := dcl.Diagnostics{{
		Severity: dcl.SeverityWarning,
		Message:  "heads up",
	}}

	got := FormatDiagnostics(diags, true)

	if !strings.Contains(got, ansiYellow+"warning"+ansiReset) {
		t.Errorf("expected yellow 'warning' label, got:\n%s", got)
	}
}

func TestFormatDiagnostics_with_suggestion(t *testing.T) {
	diags := dcl.Diagnostics{{
		Severity:   dcl.SeverityError,
		Message:    "missing endpoint",
		Suggestion: "add endpoint = \"https://...\"",
	}}

	got := FormatDiagnostics(diags, false)

	if !strings.Contains(got, "suggestion: add endpoint") {
		t.Errorf("expected suggestion line, got:\n%s", got)
	}
}

func TestFormatDiagnostics_with_range(t *testing.T) {
	diags := dcl.Diagnostics{{
		Severity: dcl.SeverityError,
		Message:  "bad value",
		Range: dcl.Range{
			Start: dcl.Pos{Filename: "config.dcl", Line: 3, Column: 5},
		},
	}}

	got := FormatDiagnostics(diags, false)

	if !strings.Contains(got, "config.dcl:3:5") {
		t.Errorf("expected file:line:col, got:\n%s", got)
	}
}

func TestFormatDiagnostics_no_color(t *testing.T) {
	diags := dcl.Diagnostics{
		{Severity: dcl.SeverityError, Message: "err"},
		{Severity: dcl.SeverityWarning, Message: "warn"},
	}

	got := FormatDiagnostics(diags, false)

	if strings.Contains(got, "\033[") {
		t.Errorf("expected no ANSI codes when color=false, got:\n%s", got)
	}
}

func TestFormatDiagnosticsJSON_structure(t *testing.T) {
	diags := dcl.Diagnostics{
		{
			Severity:   dcl.SeverityError,
			Message:    "missing field",
			Suggestion: "add it",
			Range: dcl.Range{
				Start: dcl.Pos{Filename: "test.dcl", Line: 5, Column: 10},
			},
		},
		{
			Severity: dcl.SeverityWarning,
			Message:  "deprecated",
		},
	}

	data, err := FormatDiagnosticsJSON(diags)
	if err != nil {
		t.Fatalf("FormatDiagnosticsJSON failed: %v", err)
	}

	var got jsonDiagnostics
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(got.Diagnostics) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(got.Diagnostics))
	}

	d0 := got.Diagnostics[0]
	if d0.Severity != "error" || d0.Message != "missing field" || d0.Suggestion != "add it" {
		t.Errorf("unexpected d0: %+v", d0)
	}
	if d0.Range == nil || d0.Range.File != "test.dcl" || d0.Range.Start.Line != 5 {
		t.Errorf("unexpected range: %+v", d0.Range)
	}

	d1 := got.Diagnostics[1]
	if d1.Severity != "warning" || d1.Range != nil {
		t.Errorf("unexpected d1: %+v", d1)
	}
}
