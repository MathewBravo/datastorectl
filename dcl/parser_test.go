package dcl

import "testing"

// parseString is a test helper that calls Parse and fatals on unexpected errors.
func parseString(t *testing.T, src string) *File {
	t.Helper()
	f, diags := Parse("test.dcl", []byte(src))
	if diags.HasErrors() {
		t.Fatalf("unexpected errors:\n%s", diags.Error())
	}
	return f
}

func TestParseEmptyFile(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"whitespace", "  "},
		{"newlines", "\n\n"},
		{"comment", "# comment\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseString(t, tt.src)
			if len(f.Blocks) != 0 {
				t.Errorf("expected 0 blocks, got %d", len(f.Blocks))
			}
		})
	}
}

func TestParseLabeledBlock(t *testing.T) {
	src := `resource "my_thing" { }`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	b := f.Blocks[0]
	if b.Type != "resource" {
		t.Errorf("expected type %q, got %q", "resource", b.Type)
	}
	if b.Label != "my_thing" {
		t.Errorf("expected label %q, got %q", "my_thing", b.Label)
	}
}

func TestParseUnlabeledBlock(t *testing.T) {
	src := `defaults { }`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	b := f.Blocks[0]
	if b.Type != "defaults" {
		t.Errorf("expected type %q, got %q", "defaults", b.Type)
	}
	if b.Label != "" {
		t.Errorf("expected empty label, got %q", b.Label)
	}
}

func TestParseMultipleAttributesMixedTypes(t *testing.T) {
	src := `resource "db" {
  host = "localhost"
  port = 5432
  weight = 1.5
  primary = true
  readonly = false
}`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attrs := f.Blocks[0].Attributes
	if len(attrs) != 5 {
		t.Fatalf("expected 5 attributes, got %d", len(attrs))
	}

	tests := []struct {
		key      string
		wantType string
		wantVal  interface{}
	}{
		{"host", "*dcl.LiteralString", "localhost"},
		{"port", "*dcl.LiteralInt", int64(5432)},
		{"weight", "*dcl.LiteralFloat", 1.5},
		{"primary", "*dcl.LiteralBool", true},
		{"readonly", "*dcl.LiteralBool", false},
	}

	for i, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			a := attrs[i]
			if a.Key != tt.key {
				t.Errorf("expected key %q, got %q", tt.key, a.Key)
			}
			switch v := a.Value.(type) {
			case *LiteralString:
				if tt.wantVal != v.Value {
					t.Errorf("expected %v, got %v", tt.wantVal, v.Value)
				}
			case *LiteralInt:
				if tt.wantVal != v.Value {
					t.Errorf("expected %v, got %v", tt.wantVal, v.Value)
				}
			case *LiteralFloat:
				if tt.wantVal != v.Value {
					t.Errorf("expected %v, got %v", tt.wantVal, v.Value)
				}
			case *LiteralBool:
				if tt.wantVal != v.Value {
					t.Errorf("expected %v, got %v", tt.wantVal, v.Value)
				}
			default:
				t.Errorf("unexpected expression type %T", a.Value)
			}
		})
	}
}

func TestParseCommentsIgnored(t *testing.T) {
	src := `# before block
resource "db" {
  # inside block
  host = "localhost"
}
# after block`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	if len(f.Blocks[0].Attributes) != 1 {
		t.Errorf("expected 1 attribute, got %d", len(f.Blocks[0].Attributes))
	}
}

func TestParseErrorMissingValueAfterEquals(t *testing.T) {
	src := `resource "db" {
  host =
}`
	_, diags := Parse("test.dcl", []byte(src))
	if !diags.HasErrors() {
		t.Fatal("expected errors, got none")
	}
	found := false
	for _, d := range diags {
		if d.Severity == SeverityError {
			found = true
			if !containsSubstring(d.Message, "expected value") {
				t.Errorf("expected diagnostic to mention 'expected value', got %q", d.Message)
			}
		}
	}
	if !found {
		t.Error("expected at least one error diagnostic")
	}
}

func TestParseErrorTopLevelAttribute(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantBlocks int
	}{
		{
			name:       "only attribute",
			src:        `host = "x"`,
			wantBlocks: 0,
		},
		{
			name: "attribute then block",
			src: `host = "x"
resource "db" { }`,
			wantBlocks: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, diags := Parse("test.dcl", []byte(tt.src))
			if !diags.HasErrors() {
				t.Fatal("expected errors, got none")
			}
			found := false
			for _, d := range diags {
				if containsSubstring(d.Message, "top level") {
					found = true
				}
			}
			if !found {
				t.Error("expected diagnostic about top-level attribute")
			}
			if len(f.Blocks) != tt.wantBlocks {
				t.Errorf("expected %d blocks, got %d", tt.wantBlocks, len(f.Blocks))
			}
		})
	}
}

func TestParsePositionAccuracy(t *testing.T) {
	src := "resource \"db\" {\n  host = \"localhost\"\n}"
	f := parseString(t, src)

	// File range: starts at 1:1, ends at EOF.
	if f.Rng.Start.Line != 1 || f.Rng.Start.Column != 1 {
		t.Errorf("file start: expected 1:1, got %d:%d", f.Rng.Start.Line, f.Rng.Start.Column)
	}

	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	b := f.Blocks[0]

	// Block range: starts at 1:1 ("resource"), ends after "}" on line 3.
	if b.Rng.Start.Line != 1 || b.Rng.Start.Column != 1 {
		t.Errorf("block start: expected 1:1, got %d:%d", b.Rng.Start.Line, b.Rng.Start.Column)
	}
	if b.Rng.End.Line != 3 || b.Rng.End.Column != 2 {
		t.Errorf("block end: expected 3:2, got %d:%d", b.Rng.End.Line, b.Rng.End.Column)
	}

	if len(b.Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(b.Attributes))
	}
	a := b.Attributes[0]

	// Attribute starts at line 2, col 3 ("host").
	if a.Rng.Start.Line != 2 || a.Rng.Start.Column != 3 {
		t.Errorf("attr start: expected 2:3, got %d:%d", a.Rng.Start.Line, a.Rng.Start.Column)
	}

	// Attribute value is "localhost" — string literal starts at col 10 (the opening quote).
	lit, ok := a.Value.(*LiteralString)
	if !ok {
		t.Fatalf("expected *LiteralString, got %T", a.Value)
	}
	if lit.Rng.Start.Line != 2 || lit.Rng.Start.Column != 10 {
		t.Errorf("literal start: expected 2:10, got %d:%d", lit.Rng.Start.Line, lit.Rng.Start.Column)
	}
	// "localhost" is 9 chars + 2 quotes = 11, so end col = 10 + 11 = 21
	if lit.Rng.End.Line != 2 || lit.Rng.End.Column != 21 {
		t.Errorf("literal end: expected 2:21, got %d:%d", lit.Rng.End.Line, lit.Rng.End.Column)
	}
}

func TestParseMultipleBlocks(t *testing.T) {
	src := `resource "db" {
  host = "localhost"
}

resource "cache" {
  host = "redis"
}`
	f := parseString(t, src)
	if len(f.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(f.Blocks))
	}
	if f.Blocks[0].Label != "db" {
		t.Errorf("block 0: expected label %q, got %q", "db", f.Blocks[0].Label)
	}
	if f.Blocks[1].Label != "cache" {
		t.Errorf("block 1: expected label %q, got %q", "cache", f.Blocks[1].Label)
	}
}

func TestParseErrorRecovery(t *testing.T) {
	src := `resource "db" {
  bad =
  host = "localhost"
}`
	f, diags := Parse("test.dcl", []byte(src))
	if !diags.HasErrors() {
		t.Fatal("expected errors, got none")
	}

	errorCount := 0
	for _, d := range diags {
		if d.Severity == SeverityError {
			errorCount++
		}
	}
	if errorCount != 1 {
		t.Errorf("expected 1 error, got %d", errorCount)
	}

	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attrs := f.Blocks[0].Attributes
	if len(attrs) != 1 {
		t.Fatalf("expected 1 attribute after recovery, got %d", len(attrs))
	}
	if attrs[0].Key != "host" {
		t.Errorf("expected recovered attr key %q, got %q", "host", attrs[0].Key)
	}
}

func TestParseEmptyBlock(t *testing.T) {
	src := `defaults {
  # just a comment


}`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	if len(f.Blocks[0].Attributes) != 0 {
		t.Errorf("expected 0 attributes, got %d", len(f.Blocks[0].Attributes))
	}
}

// containsSubstring checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
