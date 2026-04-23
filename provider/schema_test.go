package provider

import "testing"

// TestFieldHint_VersionFields asserts the FieldHint struct carries
// optional MinVersion and MaxVersion annotations alongside the block
// Kind. These are the inputs to the validate-time version gating from
// ADR 0009.
func TestFieldHint_VersionFields(t *testing.T) {
	h := FieldHint{
		Kind:       FieldBlockList,
		MinVersion: "8.0",
		MaxVersion: "8.4",
	}
	if h.Kind != FieldBlockList {
		t.Errorf("Kind = %v, want FieldBlockList", h.Kind)
	}
	if h.MinVersion != "8.0" || h.MaxVersion != "8.4" {
		t.Errorf("version bounds = (%q, %q), want (\"8.0\", \"8.4\")", h.MinVersion, h.MaxVersion)
	}
}

// TestFieldHint_ScalarFieldVersionOnly covers the case where a scalar
// attribute has a version constraint but no block kind (Kind = 0).
func TestFieldHint_ScalarFieldVersionOnly(t *testing.T) {
	h := FieldHint{MinVersion: "8.0"}
	if h.Kind != 0 {
		t.Errorf("Kind = %v, want 0 (no block hint)", h.Kind)
	}
	if h.MinVersion != "8.0" {
		t.Errorf("MinVersion = %q, want \"8.0\"", h.MinVersion)
	}
}

// TestCompareVersions verifies the semantic (not lexical) comparison of
// major.minor strings used by the version-gating logic.
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"8.0", "8.0", 0},
		{"8.0", "8.4", -1},
		{"8.4", "8.0", 1},
		{"5.7", "8.0", -1},
		{"8.0", "10.0", -1}, // numeric, not lexical — 10 > 8
		{"10.0", "8.0", 1},
		{"8.0.5", "8.0", 0},    // patch ignored for major.minor comparison
		{"8.4.5-rds", "8.4", 0}, // suffixes tolerated
		{"", "8.0", 0},          // empty treated as "no constraint"
		{"8.0", "", 0},
	}
	for _, c := range cases {
		got, err := CompareVersions(c.a, c.b)
		if err != nil {
			t.Errorf("CompareVersions(%q, %q) error: %v", c.a, c.b, err)
			continue
		}
		if got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestCompareVersions_MalformedInput asserts malformed version strings
// return an error so callers produce clear diagnostics.
func TestCompareVersions_MalformedInput(t *testing.T) {
	cases := []string{"not-a-version", "8", "8.x", "8.-1"}
	for _, v := range cases {
		if _, err := CompareVersions(v, "8.0"); err == nil {
			t.Errorf("CompareVersions(%q, ...) expected error, got nil", v)
		}
	}
}
