package dcl

import "testing"

func TestSeverityString(t *testing.T) {
	if got := SeverityError.String(); got != "error" {
		t.Errorf("SeverityError.String() = %q", got)
	}
	if got := SeverityWarning.String(); got != "warning" {
		t.Errorf("SeverityWarning.String() = %q", got)
	}
	if got := Severity(99).String(); got != "unknown" {
		t.Errorf("Severity(99).String() = %q", got)
	}
}

func TestDiagnosticString(t *testing.T) {
	d := Diagnostic{
		Severity: SeverityError,
		Message:  "unexpected token",
		Range:    Range{Pos{"a.dcl", 1, 5, 4}, Pos{"a.dcl", 1, 10, 9}},
	}
	want := "a.dcl:1:5: error: unexpected token"
	if got := d.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiagnosticStringWithSuggestion(t *testing.T) {
	d := Diagnostic{
		Severity:   SeverityWarning,
		Message:    "unused variable",
		Range:      Range{Pos{"", 2, 1, 10}, Pos{"", 2, 5, 14}},
		Suggestion: "remove or use the variable",
	}
	want := "2:1: warning: unused variable (remove or use the variable)"
	if got := d.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiagnosticsHasErrors(t *testing.T) {
	tests := []struct {
		name string
		ds   Diagnostics
		want bool
	}{
		{"empty", Diagnostics{}, false},
		{"warnings only", Diagnostics{
			{Severity: SeverityWarning, Message: "w1"},
			{Severity: SeverityWarning, Message: "w2"},
		}, false},
		{"errors only", Diagnostics{
			{Severity: SeverityError, Message: "e1"},
		}, true},
		{"mixed", Diagnostics{
			{Severity: SeverityWarning, Message: "w"},
			{Severity: SeverityError, Message: "e"},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ds.HasErrors(); got != tt.want {
				t.Errorf("HasErrors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiagnosticsError(t *testing.T) {
	ds := Diagnostics{
		{Severity: SeverityError, Message: "bad", Range: Range{Pos{"f.dcl", 1, 1, 0}, Pos{"f.dcl", 1, 4, 3}}},
		{Severity: SeverityWarning, Message: "warn", Range: Range{Pos{"f.dcl", 2, 1, 10}, Pos{"f.dcl", 2, 5, 14}}},
	}
	want := "f.dcl:1:1: error: bad\nf.dcl:2:1: warning: warn"
	if got := ds.Error(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiagnosticsErrorEmpty(t *testing.T) {
	if got := (Diagnostics{}).Error(); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestDiagnosticsAppend(t *testing.T) {
	a := Diagnostics{{Severity: SeverityError, Message: "a"}}
	b := Diagnostics{
		{Severity: SeverityWarning, Message: "b"},
		{Severity: SeverityError, Message: "c"},
	}
	a.Append(b)

	if len(a) != 3 {
		t.Fatalf("expected 3 diagnostics, got %d", len(a))
	}
	if a[0].Message != "a" || a[1].Message != "b" || a[2].Message != "c" {
		t.Error("Append did not preserve order")
	}
}

func TestDiagnosticsAppendEmpty(t *testing.T) {
	a := Diagnostics{{Severity: SeverityError, Message: "a"}}
	a.Append(nil)
	if len(a) != 1 {
		t.Fatalf("appending nil changed length to %d", len(a))
	}
}
