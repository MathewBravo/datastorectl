package dcl

import (
	"fmt"
	"testing"
)

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

func TestParseListSimple(t *testing.T) {
	src := `resource "app" {
  tags = ["a", "b", "c"]
}`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attrs := f.Blocks[0].Attributes
	if len(attrs) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(attrs))
	}
	list, ok := attrs[0].Value.(*ListExpr)
	if !ok {
		t.Fatalf("expected *ListExpr, got %T", attrs[0].Value)
	}
	if len(list.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(list.Elements))
	}
	for i, want := range []string{"a", "b", "c"} {
		lit, ok := list.Elements[i].(*LiteralString)
		if !ok {
			t.Errorf("element %d: expected *LiteralString, got %T", i, list.Elements[i])
			continue
		}
		if lit.Value != want {
			t.Errorf("element %d: expected %q, got %q", i, want, lit.Value)
		}
	}
}

func TestParseListEmpty(t *testing.T) {
	src := `resource "app" {
  tags = []
}`
	f := parseString(t, src)
	attrs := f.Blocks[0].Attributes
	list, ok := attrs[0].Value.(*ListExpr)
	if !ok {
		t.Fatalf("expected *ListExpr, got %T", attrs[0].Value)
	}
	if len(list.Elements) != 0 {
		t.Errorf("expected 0 elements, got %d", len(list.Elements))
	}
}

func TestParseListTrailingComma(t *testing.T) {
	src := `resource "app" {
  ports = [8080, 8443,]
}`
	f := parseString(t, src)
	attrs := f.Blocks[0].Attributes
	list, ok := attrs[0].Value.(*ListExpr)
	if !ok {
		t.Fatalf("expected *ListExpr, got %T", attrs[0].Value)
	}
	if len(list.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(list.Elements))
	}
	for i, want := range []int64{8080, 8443} {
		lit, ok := list.Elements[i].(*LiteralInt)
		if !ok {
			t.Errorf("element %d: expected *LiteralInt, got %T", i, list.Elements[i])
			continue
		}
		if lit.Value != want {
			t.Errorf("element %d: expected %d, got %d", i, want, lit.Value)
		}
	}
}

func TestParseListMultiline(t *testing.T) {
	src := "resource \"app\" {\n  ports = [\n    8080,\n    8443\n  ]\n}"
	f := parseString(t, src)
	attrs := f.Blocks[0].Attributes
	list, ok := attrs[0].Value.(*ListExpr)
	if !ok {
		t.Fatalf("expected *ListExpr, got %T", attrs[0].Value)
	}
	if len(list.Elements) != 2 {
		t.Errorf("expected 2 elements, got %d", len(list.Elements))
	}
}

func TestParseMapSimple(t *testing.T) {
	src := `resource "app" {
  labels = { env = "prod", tier = "web" }
}`
	f := parseString(t, src)
	attrs := f.Blocks[0].Attributes
	m, ok := attrs[0].Value.(*MapExpr)
	if !ok {
		t.Fatalf("expected *MapExpr, got %T", attrs[0].Value)
	}
	if len(m.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(m.Keys))
	}
	wantKeys := []string{"env", "tier"}
	wantVals := []string{"prod", "web"}
	for i := range wantKeys {
		if m.Keys[i] != wantKeys[i] {
			t.Errorf("key %d: expected %q, got %q", i, wantKeys[i], m.Keys[i])
		}
		lit, ok := m.Values[i].(*LiteralString)
		if !ok {
			t.Errorf("value %d: expected *LiteralString, got %T", i, m.Values[i])
			continue
		}
		if lit.Value != wantVals[i] {
			t.Errorf("value %d: expected %q, got %q", i, wantVals[i], lit.Value)
		}
	}
}

func TestParseMapEmpty(t *testing.T) {
	src := `resource "app" {
  meta = {}
}`
	f := parseString(t, src)
	attrs := f.Blocks[0].Attributes
	m, ok := attrs[0].Value.(*MapExpr)
	if !ok {
		t.Fatalf("expected *MapExpr, got %T", attrs[0].Value)
	}
	if len(m.Keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(m.Keys))
	}
}

func TestParseMapMultilineTrailingComma(t *testing.T) {
	src := "resource \"app\" {\n  labels = {\n    env = \"prod\",\n    tier = \"web\",\n  }\n}"
	f := parseString(t, src)
	attrs := f.Blocks[0].Attributes
	m, ok := attrs[0].Value.(*MapExpr)
	if !ok {
		t.Fatalf("expected *MapExpr, got %T", attrs[0].Value)
	}
	if len(m.Keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(m.Keys))
	}
}

func TestParseNestedListOfMaps(t *testing.T) {
	src := "resource \"app\" {\n  servers = [{ host = \"a\" }, { host = \"b\" }]\n}"
	f := parseString(t, src)
	attrs := f.Blocks[0].Attributes
	list, ok := attrs[0].Value.(*ListExpr)
	if !ok {
		t.Fatalf("expected *ListExpr, got %T", attrs[0].Value)
	}
	if len(list.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(list.Elements))
	}
	for i, want := range []string{"a", "b"} {
		m, ok := list.Elements[i].(*MapExpr)
		if !ok {
			t.Errorf("element %d: expected *MapExpr, got %T", i, list.Elements[i])
			continue
		}
		if len(m.Keys) != 1 || m.Keys[0] != "host" {
			t.Errorf("element %d: expected key 'host', got %v", i, m.Keys)
			continue
		}
		lit, ok := m.Values[0].(*LiteralString)
		if !ok {
			t.Errorf("element %d: expected *LiteralString value, got %T", i, m.Values[0])
			continue
		}
		if lit.Value != want {
			t.Errorf("element %d: expected value %q, got %q", i, want, lit.Value)
		}
	}
}

func TestParseNestedMapOfLists(t *testing.T) {
	src := "resource \"app\" {\n  config = { ports = [80], hosts = [\"a\"] }\n}"
	f := parseString(t, src)
	attrs := f.Blocks[0].Attributes
	m, ok := attrs[0].Value.(*MapExpr)
	if !ok {
		t.Fatalf("expected *MapExpr, got %T", attrs[0].Value)
	}
	if len(m.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(m.Keys))
	}
	for i, key := range []string{"ports", "hosts"} {
		if m.Keys[i] != key {
			t.Errorf("key %d: expected %q, got %q", i, key, m.Keys[i])
		}
		if _, ok := m.Values[i].(*ListExpr); !ok {
			t.Errorf("value %d: expected *ListExpr, got %T", i, m.Values[i])
		}
	}
}

func TestParseListUnterminated(t *testing.T) {
	src := `resource "app" {
  tags = ["a", "b"
}`
	_, diags := Parse("test.dcl", []byte(src))
	if !diags.HasErrors() {
		t.Fatal("expected errors, got none")
	}
	found := false
	for _, d := range diags {
		if d.Severity == SeverityError && containsSubstring(d.Message, "RBracket") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic mentioning RBracket, got: %s", diags.Error())
	}
}

func TestParseMapMissingEquals(t *testing.T) {
	src := `resource "app" {
  labels = { foo "bar" }
}`
	_, diags := Parse("test.dcl", []byte(src))
	if !diags.HasErrors() {
		t.Fatal("expected errors, got none")
	}
	found := false
	for _, d := range diags {
		if d.Severity == SeverityError && containsSubstring(d.Message, "Equals") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic mentioning Equals, got: %s", diags.Error())
	}
}

func TestParseNestedLabeledBlock(t *testing.T) {
	src := `policy "p1" {
  state "hot" {
    priority = 1
  }
}`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	outer := f.Blocks[0]
	if outer.Type != "policy" || outer.Label != "p1" {
		t.Errorf("outer: expected policy/p1, got %s/%s", outer.Type, outer.Label)
	}
	if len(outer.Blocks) != 1 {
		t.Fatalf("expected 1 nested block, got %d", len(outer.Blocks))
	}
	inner := outer.Blocks[0]
	if inner.Type != "state" || inner.Label != "hot" {
		t.Errorf("inner: expected state/hot, got %s/%s", inner.Type, inner.Label)
	}
	if len(inner.Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(inner.Attributes))
	}
	if inner.Attributes[0].Key != "priority" {
		t.Errorf("expected key %q, got %q", "priority", inner.Attributes[0].Key)
	}
}

func TestParseNestedUnlabeledBlock(t *testing.T) {
	src := `state "hot" {
  transition {
    dest = "warm"
  }
}`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	outer := f.Blocks[0]
	if len(outer.Blocks) != 1 {
		t.Fatalf("expected 1 nested block, got %d", len(outer.Blocks))
	}
	inner := outer.Blocks[0]
	if inner.Type != "transition" {
		t.Errorf("expected type %q, got %q", "transition", inner.Type)
	}
	if inner.Label != "" {
		t.Errorf("expected empty label, got %q", inner.Label)
	}
	if len(inner.Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(inner.Attributes))
	}
	if inner.Attributes[0].Key != "dest" {
		t.Errorf("expected key %q, got %q", "dest", inner.Attributes[0].Key)
	}
}

func TestParseMixedAttrsAndBlocks(t *testing.T) {
	src := `resource "db" {
  host = "localhost"
  replica {
    host = "replica1"
  }
  port = 5432
}`
	f := parseString(t, src)
	b := f.Blocks[0]
	if len(b.Attributes) != 2 {
		t.Fatalf("expected 2 attributes, got %d", len(b.Attributes))
	}
	if b.Attributes[0].Key != "host" {
		t.Errorf("attr 0: expected %q, got %q", "host", b.Attributes[0].Key)
	}
	if b.Attributes[1].Key != "port" {
		t.Errorf("attr 1: expected %q, got %q", "port", b.Attributes[1].Key)
	}
	if len(b.Blocks) != 1 {
		t.Fatalf("expected 1 nested block, got %d", len(b.Blocks))
	}
	if b.Blocks[0].Type != "replica" {
		t.Errorf("nested block: expected type %q, got %q", "replica", b.Blocks[0].Type)
	}
}

func TestParseThreeLevelNesting(t *testing.T) {
	src := `a {
  b {
    c {
      x = 1
    }
  }
}`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	a := f.Blocks[0]
	if a.Type != "a" {
		t.Errorf("expected type %q, got %q", "a", a.Type)
	}
	if len(a.Blocks) != 1 {
		t.Fatalf("a: expected 1 nested block, got %d", len(a.Blocks))
	}
	b := a.Blocks[0]
	if b.Type != "b" {
		t.Errorf("expected type %q, got %q", "b", b.Type)
	}
	if len(b.Blocks) != 1 {
		t.Fatalf("b: expected 1 nested block, got %d", len(b.Blocks))
	}
	c := b.Blocks[0]
	if c.Type != "c" {
		t.Errorf("expected type %q, got %q", "c", c.Type)
	}
	if len(c.Attributes) != 1 {
		t.Fatalf("c: expected 1 attribute, got %d", len(c.Attributes))
	}
	if c.Attributes[0].Key != "x" {
		t.Errorf("expected key %q, got %q", "x", c.Attributes[0].Key)
	}
}

func TestParseSiblingBlocks(t *testing.T) {
	src := `outer {
  a { }
  b { }
}`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	outer := f.Blocks[0]
	if len(outer.Blocks) != 2 {
		t.Fatalf("expected 2 nested blocks, got %d", len(outer.Blocks))
	}
	if outer.Blocks[0].Type != "a" {
		t.Errorf("block 0: expected type %q, got %q", "a", outer.Blocks[0].Type)
	}
	if outer.Blocks[1].Type != "b" {
		t.Errorf("block 1: expected type %q, got %q", "b", outer.Blocks[1].Type)
	}
}

func TestParseMapNotBlock(t *testing.T) {
	src := `resource "r" {
  config = { env = "prod" }
}`
	f := parseString(t, src)
	b := f.Blocks[0]
	if len(b.Blocks) != 0 {
		t.Errorf("expected 0 nested blocks, got %d", len(b.Blocks))
	}
	if len(b.Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(b.Attributes))
	}
	if _, ok := b.Attributes[0].Value.(*MapExpr); !ok {
		t.Errorf("expected *MapExpr, got %T", b.Attributes[0].Value)
	}
}

func TestParseFullISMStructure(t *testing.T) {
	src := `policy "rollover" {
  description = "Rollover and shrink"

  state "hot" {
    priority = 100
    transition {
      condition = "min_index_age"
      value = "1d"
      dest = "warm"
    }
  }

  state "warm" {
    priority = 50
    actions {
      shrink = true
    }
    transition {
      condition = "min_index_age"
      value = "30d"
      dest = "delete"
    }
  }

  state "delete" {
    actions {
      delete = true
    }
  }
}`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	policy := f.Blocks[0]
	if policy.Type != "policy" || policy.Label != "rollover" {
		t.Errorf("expected policy/rollover, got %s/%s", policy.Type, policy.Label)
	}
	if len(policy.Attributes) != 1 {
		t.Errorf("expected 1 policy attribute, got %d", len(policy.Attributes))
	}
	if len(policy.Blocks) != 3 {
		t.Fatalf("expected 3 state blocks, got %d", len(policy.Blocks))
	}

	// Validate state names.
	wantStates := []string{"hot", "warm", "delete"}
	for i, want := range wantStates {
		if policy.Blocks[i].Type != "state" {
			t.Errorf("block %d: expected type %q, got %q", i, "state", policy.Blocks[i].Type)
		}
		if policy.Blocks[i].Label != want {
			t.Errorf("block %d: expected label %q, got %q", i, want, policy.Blocks[i].Label)
		}
	}

	// Hot state: 1 attr (priority), 1 block (transition).
	hot := policy.Blocks[0]
	if len(hot.Attributes) != 1 {
		t.Errorf("hot: expected 1 attribute, got %d", len(hot.Attributes))
	}
	if len(hot.Blocks) != 1 {
		t.Fatalf("hot: expected 1 nested block, got %d", len(hot.Blocks))
	}
	if hot.Blocks[0].Type != "transition" {
		t.Errorf("hot nested: expected type %q, got %q", "transition", hot.Blocks[0].Type)
	}
	if len(hot.Blocks[0].Attributes) != 3 {
		t.Errorf("hot transition: expected 3 attributes, got %d", len(hot.Blocks[0].Attributes))
	}

	// Warm state: 1 attr, 2 blocks (actions + transition).
	warm := policy.Blocks[1]
	if len(warm.Attributes) != 1 {
		t.Errorf("warm: expected 1 attribute, got %d", len(warm.Attributes))
	}
	if len(warm.Blocks) != 2 {
		t.Fatalf("warm: expected 2 nested blocks, got %d", len(warm.Blocks))
	}
	if warm.Blocks[0].Type != "actions" {
		t.Errorf("warm block 0: expected type %q, got %q", "actions", warm.Blocks[0].Type)
	}
	if warm.Blocks[1].Type != "transition" {
		t.Errorf("warm block 1: expected type %q, got %q", "transition", warm.Blocks[1].Type)
	}

	// Delete state: 0 attrs, 1 block (actions).
	del := policy.Blocks[2]
	if len(del.Attributes) != 0 {
		t.Errorf("delete: expected 0 attributes, got %d", len(del.Attributes))
	}
	if len(del.Blocks) != 1 {
		t.Fatalf("delete: expected 1 nested block, got %d", len(del.Blocks))
	}
	if del.Blocks[0].Type != "actions" {
		t.Errorf("delete block 0: expected type %q, got %q", "actions", del.Blocks[0].Type)
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

func TestParseBareIdentifier(t *testing.T) {
	f := parseString(t, `resource "r" { context = prod_opensearch }`)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attr := f.Blocks[0].Attributes[0]
	ident, ok := attr.Value.(*Identifier)
	if !ok {
		t.Fatalf("expected *Identifier, got %T", attr.Value)
	}
	if ident.Name != "prod_opensearch" {
		t.Errorf("expected Name %q, got %q", "prod_opensearch", ident.Name)
	}
}

func TestParseTwoPartReference(t *testing.T) {
	f := parseString(t, `role_mapping "rm" { role = opensearch_role.log_reader }`)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attr := f.Blocks[0].Attributes[0]
	ref, ok := attr.Value.(*Reference)
	if !ok {
		t.Fatalf("expected *Reference, got %T", attr.Value)
	}
	if len(ref.Parts) != 2 || ref.Parts[0] != "opensearch_role" || ref.Parts[1] != "log_reader" {
		t.Errorf("expected [opensearch_role log_reader], got %v", ref.Parts)
	}
}

func TestParseFourPartReference(t *testing.T) {
	f := parseString(t, `resource "r" { path = a.b.c.d }`)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attr := f.Blocks[0].Attributes[0]
	ref, ok := attr.Value.(*Reference)
	if !ok {
		t.Fatalf("expected *Reference, got %T", attr.Value)
	}
	expected := []string{"a", "b", "c", "d"}
	if len(ref.Parts) != len(expected) {
		t.Fatalf("expected %d parts, got %d", len(expected), len(ref.Parts))
	}
	for i, p := range expected {
		if ref.Parts[i] != p {
			t.Errorf("part[%d]: expected %q, got %q", i, p, ref.Parts[i])
		}
	}
}

func TestParseReferenceInList(t *testing.T) {
	f := parseString(t, `resource "r" { refs = [a.b, c] }`)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attr := f.Blocks[0].Attributes[0]
	list, ok := attr.Value.(*ListExpr)
	if !ok {
		t.Fatalf("expected *ListExpr, got %T", attr.Value)
	}
	if len(list.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(list.Elements))
	}
	ref, ok := list.Elements[0].(*Reference)
	if !ok {
		t.Fatalf("element[0]: expected *Reference, got %T", list.Elements[0])
	}
	if len(ref.Parts) != 2 || ref.Parts[0] != "a" || ref.Parts[1] != "b" {
		t.Errorf("element[0]: expected [a b], got %v", ref.Parts)
	}
	ident, ok := list.Elements[1].(*Identifier)
	if !ok {
		t.Fatalf("element[1]: expected *Identifier, got %T", list.Elements[1])
	}
	if ident.Name != "c" {
		t.Errorf("element[1]: expected %q, got %q", "c", ident.Name)
	}
}

func TestParseTrailingDotError(t *testing.T) {
	_, diags := Parse("test.dcl", []byte(`resource "r" { ref = foo. }`))
	if !diags.HasErrors() {
		t.Fatal("expected error for trailing dot")
	}
	found := false
	for _, d := range diags {
		if containsSubstring(d.Message, "expected identifier after '.'") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error mentioning \"expected identifier after '.'\", got: %s", diags.Error())
	}
}

func TestParseLeadingDotError(t *testing.T) {
	_, diags := Parse("test.dcl", []byte(`resource "r" { ref = .foo }`))
	if !diags.HasErrors() {
		t.Fatal("expected error for leading dot")
	}
	found := false
	for _, d := range diags {
		if containsSubstring(d.Message, "expected value") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error mentioning \"expected value\", got: %s", diags.Error())
	}
}

func TestParseRoleMappingExample(t *testing.T) {
	src := `
role_mapping "readall" {
  cluster_permissions = ["cluster_composite_ops_ro"]
  index_permissions {
    index_patterns  = ["*"]
    allowed_actions = ["read"]
  }
  backend_roles = [opensearch_role.log_reader, "admin"]
  description   = "read-only access"
}
`
	f := parseString(t, src)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	b := f.Blocks[0]
	if b.Type != "role_mapping" || b.Label != "readall" {
		t.Errorf("unexpected block type/label: %s %q", b.Type, b.Label)
	}

	// Find backend_roles attribute
	var backendRoles *ListExpr
	for _, attr := range b.Attributes {
		if attr.Key == "backend_roles" {
			var ok bool
			backendRoles, ok = attr.Value.(*ListExpr)
			if !ok {
				t.Fatalf("backend_roles: expected *ListExpr, got %T", attr.Value)
			}
		}
	}
	if backendRoles == nil {
		t.Fatal("backend_roles attribute not found")
	}
	if len(backendRoles.Elements) != 2 {
		t.Fatalf("expected 2 elements in backend_roles, got %d", len(backendRoles.Elements))
	}
	ref, ok := backendRoles.Elements[0].(*Reference)
	if !ok {
		t.Fatalf("element[0]: expected *Reference, got %T", backendRoles.Elements[0])
	}
	if len(ref.Parts) != 2 || ref.Parts[0] != "opensearch_role" || ref.Parts[1] != "log_reader" {
		t.Errorf("element[0]: expected [opensearch_role log_reader], got %v", ref.Parts)
	}
	str, ok := backendRoles.Elements[1].(*LiteralString)
	if !ok {
		t.Fatalf("element[1]: expected *LiteralString, got %T", backendRoles.Elements[1])
	}
	if str.Value != "admin" {
		t.Errorf("element[1]: expected %q, got %q", "admin", str.Value)
	}
}

func TestParseFunctionCallTwoArgs(t *testing.T) {
	f := parseString(t, `resource "r" { password = secret("env", "DB_PASS") }`)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attr := f.Blocks[0].Attributes[0]
	fc, ok := attr.Value.(*FunctionCall)
	if !ok {
		t.Fatalf("expected *FunctionCall, got %T", attr.Value)
	}
	if fc.Name != "secret" {
		t.Errorf("expected Name %q, got %q", "secret", fc.Name)
	}
	if len(fc.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(fc.Args))
	}
	arg0, ok := fc.Args[0].(*LiteralString)
	if !ok {
		t.Fatalf("arg[0]: expected *LiteralString, got %T", fc.Args[0])
	}
	if arg0.Value != "env" {
		t.Errorf("arg[0]: expected %q, got %q", "env", arg0.Value)
	}
	arg1, ok := fc.Args[1].(*LiteralString)
	if !ok {
		t.Fatalf("arg[1]: expected *LiteralString, got %T", fc.Args[1])
	}
	if arg1.Value != "DB_PASS" {
		t.Errorf("arg[1]: expected %q, got %q", "DB_PASS", arg1.Value)
	}
}

func TestParseFunctionCallSingleArg(t *testing.T) {
	f := parseString(t, `resource "r" { age = min_index_age("7d") }`)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attr := f.Blocks[0].Attributes[0]
	fc, ok := attr.Value.(*FunctionCall)
	if !ok {
		t.Fatalf("expected *FunctionCall, got %T", attr.Value)
	}
	if fc.Name != "min_index_age" {
		t.Errorf("expected Name %q, got %q", "min_index_age", fc.Name)
	}
	if len(fc.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(fc.Args))
	}
	arg0, ok := fc.Args[0].(*LiteralString)
	if !ok {
		t.Fatalf("arg[0]: expected *LiteralString, got %T", fc.Args[0])
	}
	if arg0.Value != "7d" {
		t.Errorf("arg[0]: expected %q, got %q", "7d", arg0.Value)
	}
}

func TestParseFunctionCallEmpty(t *testing.T) {
	f := parseString(t, `resource "r" { v = noop() }`)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attr := f.Blocks[0].Attributes[0]
	fc, ok := attr.Value.(*FunctionCall)
	if !ok {
		t.Fatalf("expected *FunctionCall, got %T", attr.Value)
	}
	if fc.Name != "noop" {
		t.Errorf("expected Name %q, got %q", "noop", fc.Name)
	}
	if len(fc.Args) != 0 {
		t.Errorf("expected 0 args, got %d", len(fc.Args))
	}
}

func TestParseFunctionCallNested(t *testing.T) {
	f := parseString(t, `resource "r" { v = outer(inner("x")) }`)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attr := f.Blocks[0].Attributes[0]
	fc, ok := attr.Value.(*FunctionCall)
	if !ok {
		t.Fatalf("expected *FunctionCall, got %T", attr.Value)
	}
	if fc.Name != "outer" {
		t.Errorf("expected Name %q, got %q", "outer", fc.Name)
	}
	if len(fc.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(fc.Args))
	}
	inner, ok := fc.Args[0].(*FunctionCall)
	if !ok {
		t.Fatalf("arg[0]: expected *FunctionCall, got %T", fc.Args[0])
	}
	if inner.Name != "inner" {
		t.Errorf("inner: expected Name %q, got %q", "inner", inner.Name)
	}
	if len(inner.Args) != 1 {
		t.Fatalf("inner: expected 1 arg, got %d", len(inner.Args))
	}
	s, ok := inner.Args[0].(*LiteralString)
	if !ok {
		t.Fatalf("inner arg[0]: expected *LiteralString, got %T", inner.Args[0])
	}
	if s.Value != "x" {
		t.Errorf("inner arg[0]: expected %q, got %q", "x", s.Value)
	}
}

func TestParseFunctionCallTrailingComma(t *testing.T) {
	f := parseString(t, `resource "r" { v = f("a",) }`)
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	attr := f.Blocks[0].Attributes[0]
	fc, ok := attr.Value.(*FunctionCall)
	if !ok {
		t.Fatalf("expected *FunctionCall, got %T", attr.Value)
	}
	if fc.Name != "f" {
		t.Errorf("expected Name %q, got %q", "f", fc.Name)
	}
	if len(fc.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(fc.Args))
	}
}

func TestParseFunctionCallUnterminated(t *testing.T) {
	_, diags := Parse("test.dcl", []byte(`resource "r" { v = secret("x" }`))
	if !diags.HasErrors() {
		t.Fatal("expected error for unterminated function call")
	}
	found := false
	for _, d := range diags {
		if containsSubstring(d.Message, "expected ',' or ')'") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error mentioning \"expected ',' or ')'\", got: %s", diags.Error())
	}
}

func TestParseFunctionCallMissingComma(t *testing.T) {
	_, diags := Parse("test.dcl", []byte(`resource "r" { v = secret("a" "b") }`))
	if !diags.HasErrors() {
		t.Fatal("expected error for missing comma in function call")
	}
	found := false
	for _, d := range diags {
		if containsSubstring(d.Message, "expected ',' or ')'") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error mentioning \"expected ',' or ')'\", got: %s", diags.Error())
	}
}

// --- Error recovery tests ---

func TestRecovery_ErrorInFirstBlock_SecondBlockValid(t *testing.T) {
	src := `resource "broken" {
  name = )
}
resource "good" {
  name = "valid"
}`
	f, diags := Parse("test.dcl", []byte(src))
	if len(f.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(f.Blocks))
	}
	if !diags.HasErrors() {
		t.Fatal("expected errors, got none")
	}
	good := f.Blocks[1]
	if good.Type != "resource" || good.Label != "good" {
		t.Errorf("expected second block type=resource label=good, got type=%q label=%q", good.Type, good.Label)
	}
	if len(good.Attributes) != 1 || good.Attributes[0].Key != "name" {
		t.Errorf("expected second block to have 1 attr 'name', got %d attrs", len(good.Attributes))
	}
}

func TestRecovery_ErrorInNestedBlock_ParentAndSiblingsParsed(t *testing.T) {
	src := `outer "o" {
  broken {
    name = )
  }
  valid {
    name = "ok"
  }
}`
	f, diags := Parse("test.dcl", []byte(src))
	if !diags.HasErrors() {
		t.Fatal("expected errors, got none")
	}
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 outer block, got %d", len(f.Blocks))
	}
	outer := f.Blocks[0]
	if len(outer.Blocks) != 2 {
		t.Fatalf("expected 2 nested blocks, got %d", len(outer.Blocks))
	}
	sibling := outer.Blocks[1]
	if sibling.Type != "valid" {
		t.Errorf("expected sibling type 'valid', got %q", sibling.Type)
	}
	if len(sibling.Attributes) != 1 || sibling.Attributes[0].Key != "name" {
		t.Errorf("expected sibling to have attr 'name'")
	}
}

func TestRecovery_ThreeErrorsAllReported(t *testing.T) {
	src := `a "a1" {
  x = )
}
b "b1" {
  y = )
}
c "c1" {
  z = )
}`
	f, diags := Parse("test.dcl", []byte(src))
	if len(f.Blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(f.Blocks))
	}
	errCount := 0
	for _, d := range diags {
		if d.Severity == SeverityError {
			errCount++
		}
	}
	if errCount != 3 {
		t.Errorf("expected 3 errors, got %d", errCount)
	}
}

func TestRecovery_DeepNesting(t *testing.T) {
	src := `a "a1" {
  b "b1" {
    c {
      x = )
    }
    d {
      name = "ok"
    }
  }
}`
	f, diags := Parse("test.dcl", []byte(src))
	if !diags.HasErrors() {
		t.Fatal("expected errors, got none")
	}
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 top block, got %d", len(f.Blocks))
	}
	a := f.Blocks[0]
	if len(a.Blocks) != 1 {
		t.Fatalf("expected 1 nested block in a, got %d", len(a.Blocks))
	}
	b := a.Blocks[0]
	if len(b.Blocks) != 2 {
		t.Fatalf("expected 2 nested blocks in b, got %d", len(b.Blocks))
	}
	d := b.Blocks[1]
	if d.Type != "d" {
		t.Errorf("expected block type 'd', got %q", d.Type)
	}
	if len(d.Attributes) != 1 || d.Attributes[0].Key != "name" {
		t.Errorf("expected d to have attr 'name'")
	}
}

func TestRecovery_ErrorCap(t *testing.T) {
	// Generate 25 blocks each with an error.
	src := ""
	for i := 0; i < 25; i++ {
		src += fmt.Sprintf("block%d {\n  x = )\n}\n", i)
	}
	_, diags := Parse("test.dcl", []byte(src))
	errCount := 0
	for _, d := range diags {
		if d.Severity == SeverityError {
			errCount++
		}
	}
	// 20 real errors + 1 sentinel "too many errors"
	if errCount != 21 {
		t.Errorf("expected 21 error diagnostics (20 + sentinel), got %d", errCount)
	}
	last := diags[len(diags)-1]
	if !containsSubstring(last.Message, "too many errors") {
		t.Errorf("expected last diagnostic to say 'too many errors', got %q", last.Message)
	}
}

func TestRecovery_MissingClosingBraceAtEOF(t *testing.T) {
	src := `resource "r" {
  name = "hello"
`
	f, diags := Parse("test.dcl", []byte(src))
	if !diags.HasErrors() {
		t.Fatal("expected errors, got none")
	}
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 partial block, got %d", len(f.Blocks))
	}
	b := f.Blocks[0]
	if b.Type != "resource" || b.Label != "r" {
		t.Errorf("expected type=resource label=r, got type=%q label=%q", b.Type, b.Label)
	}
	if len(b.Attributes) != 1 || b.Attributes[0].Key != "name" {
		t.Errorf("expected 1 attr 'name', got %d attrs", len(b.Attributes))
	}
	found := false
	for _, d := range diags {
		if containsSubstring(d.Message, "missing closing '}'") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error about missing closing '}', got: %s", diags.Error())
	}
}

func TestRecovery_ValidFileUnaffected(t *testing.T) {
	src := `ism "security_baseline" {
  version = "1.0"
  control "access" {
    severity = "high"
    rule {
      description = "require MFA"
      enabled = true
    }
  }
  control "logging" {
    severity = "medium"
  }
}`
	f, diags := Parse("test.dcl", []byte(src))
	if diags.HasErrors() {
		t.Fatalf("expected no errors, got: %s", diags.Error())
	}
	if len(f.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(f.Blocks))
	}
	ism := f.Blocks[0]
	if ism.Type != "ism" || ism.Label != "security_baseline" {
		t.Errorf("unexpected top block: type=%q label=%q", ism.Type, ism.Label)
	}
	if len(ism.Attributes) != 1 {
		t.Errorf("expected 1 attr, got %d", len(ism.Attributes))
	}
	if len(ism.Blocks) != 2 {
		t.Fatalf("expected 2 nested blocks, got %d", len(ism.Blocks))
	}
	access := ism.Blocks[0]
	if access.Type != "control" || access.Label != "access" {
		t.Errorf("unexpected first nested: type=%q label=%q", access.Type, access.Label)
	}
	if len(access.Blocks) != 1 || access.Blocks[0].Type != "rule" {
		t.Errorf("expected 1 rule block inside access")
	}
	logging := ism.Blocks[1]
	if logging.Type != "control" || logging.Label != "logging" {
		t.Errorf("unexpected second nested: type=%q label=%q", logging.Type, logging.Label)
	}
}
