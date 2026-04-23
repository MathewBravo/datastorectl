package engine

import (
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// helper: build a resource block with a single attribute
func attrBlock(typ, label, attrKey string, attrRng dcl.Range) dcl.Block {
	return dcl.Block{
		Type:  typ,
		Label: label,
		Attributes: []dcl.Attribute{
			{Key: attrKey, Value: &dcl.LiteralBool{Value: false}, Rng: attrRng},
		},
	}
}

func TestValidateBlockVersions_NoConstraintsPass(t *testing.T) {
	blocks := []dcl.Block{attrBlock("mysql_user", "app", "account_locked", dcl.Range{})}
	schemas := map[string]provider.Schema{
		"mysql_user": {Fields: map[string]provider.FieldHint{}},
	}
	diags := ValidateBlockVersions(blocks, schemas, func(dcl.Block) string { return "8.4" })
	if diags.HasErrors() {
		t.Errorf("expected no errors, got: %v", diags)
	}
}

func TestValidateBlockVersions_EmptyVersionSkips(t *testing.T) {
	blocks := []dcl.Block{attrBlock("mysql_user", "app", "account_locked", dcl.Range{})}
	schemas := map[string]provider.Schema{
		"mysql_user": {Fields: map[string]provider.FieldHint{
			"account_locked": {MinVersion: "8.0"},
		}},
	}
	// Empty version signals "no context version available" — skip checks.
	diags := ValidateBlockVersions(blocks, schemas, func(dcl.Block) string { return "" })
	if diags.HasErrors() {
		t.Errorf("expected no errors when version is empty, got: %v", diags)
	}
}

func TestValidateBlockVersions_MinVersionViolated(t *testing.T) {
	blocks := []dcl.Block{attrBlock("mysql_user", "app", "account_locked", dcl.Range{})}
	schemas := map[string]provider.Schema{
		"mysql_user": {Fields: map[string]provider.FieldHint{
			"account_locked": {MinVersion: "8.0"},
		}},
	}
	diags := ValidateBlockVersions(blocks, schemas, func(dcl.Block) string { return "5.7" })
	if !diags.HasErrors() {
		t.Fatalf("expected an error, got none")
	}
	msg := diags[0].Message
	if !strings.Contains(msg, "account_locked") || !strings.Contains(msg, "8.0") || !strings.Contains(msg, "5.7") {
		t.Errorf("expected diagnostic to mention field, MinVersion, and declared version; got: %q", msg)
	}
}

func TestValidateBlockVersions_MaxVersionViolated(t *testing.T) {
	blocks := []dcl.Block{attrBlock("mysql_user", "app", "old_plugin", dcl.Range{})}
	schemas := map[string]provider.Schema{
		"mysql_user": {Fields: map[string]provider.FieldHint{
			"old_plugin": {MaxVersion: "8.0"},
		}},
	}
	diags := ValidateBlockVersions(blocks, schemas, func(dcl.Block) string { return "8.4" })
	if !diags.HasErrors() {
		t.Fatalf("expected an error, got none")
	}
	if !strings.Contains(diags[0].Message, "old_plugin") {
		t.Errorf("expected diagnostic to mention field; got: %q", diags[0].Message)
	}
}

func TestValidateBlockVersions_InsideBoundsPass(t *testing.T) {
	blocks := []dcl.Block{attrBlock("mysql_user", "app", "windowed", dcl.Range{})}
	schemas := map[string]provider.Schema{
		"mysql_user": {Fields: map[string]provider.FieldHint{
			"windowed": {MinVersion: "8.0", MaxVersion: "8.4"},
		}},
	}
	diags := ValidateBlockVersions(blocks, schemas, func(dcl.Block) string { return "8.2" })
	if diags.HasErrors() {
		t.Errorf("expected no errors (inside bounds), got: %v", diags)
	}
}

func TestValidateBlockVersions_UnknownFieldIgnored(t *testing.T) {
	blocks := []dcl.Block{attrBlock("mysql_user", "app", "completely_unknown", dcl.Range{})}
	schemas := map[string]provider.Schema{
		"mysql_user": {Fields: map[string]provider.FieldHint{}},
	}
	diags := ValidateBlockVersions(blocks, schemas, func(dcl.Block) string { return "8.4" })
	if diags.HasErrors() {
		t.Errorf("expected no errors when field has no schema entry, got: %v", diags)
	}
}

func TestValidateBlockVersions_NestedBlockConstraint(t *testing.T) {
	blocks := []dcl.Block{
		{
			Type:  "mysql_user",
			Label: "app",
			Blocks: []dcl.Block{
				{Type: "restricted_sub_block", Rng: dcl.Range{}},
			},
		},
	}
	schemas := map[string]provider.Schema{
		"mysql_user": {Fields: map[string]provider.FieldHint{
			"restricted_sub_block": {Kind: provider.FieldBlockList, MinVersion: "8.4"},
		}},
	}
	diags := ValidateBlockVersions(blocks, schemas, func(dcl.Block) string { return "8.0" })
	if !diags.HasErrors() {
		t.Fatalf("expected error for nested block below MinVersion, got none")
	}
	if !strings.Contains(diags[0].Message, "restricted_sub_block") {
		t.Errorf("expected nested block name in diagnostic; got: %q", diags[0].Message)
	}
}

func TestValidateBlockVersions_MultipleResourcesDifferentVersions(t *testing.T) {
	blocks := []dcl.Block{
		attrBlock("mysql_user", "a", "newfield", dcl.Range{}),
		attrBlock("mysql_user", "b", "newfield", dcl.Range{}),
	}
	schemas := map[string]provider.Schema{
		"mysql_user": {Fields: map[string]provider.FieldHint{
			"newfield": {MinVersion: "8.4"},
		}},
	}
	// a → 8.0 (fails), b → 8.4 (passes)
	diags := ValidateBlockVersions(blocks, schemas, func(b dcl.Block) string {
		if b.Label == "a" {
			return "8.0"
		}
		return "8.4"
	})
	if len(diags) != 1 {
		t.Fatalf("expected exactly 1 diagnostic, got %d: %v", len(diags), diags)
	}
}
