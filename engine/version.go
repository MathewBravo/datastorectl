package engine

import (
	"fmt"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// ValidateBlockVersions walks each declared block and emits diagnostics
// for attributes or nested blocks whose schema version constraints
// don't match the declared target version for that block's context.
//
// versionFor returns the declared version string for a block (e.g.
// "8.0" or "8.4"). An empty string signals "no version constraint
// applies" and skips checks for that block. This lets the OpenSearch
// provider (which has no version-targeting model) coexist with MySQL.
//
// Unknown fields (not declared in the schema) are ignored — the schema
// authority model is "silence = unconstrained."
func ValidateBlockVersions(
	blocks []dcl.Block,
	schemas map[string]provider.Schema,
	versionFor func(dcl.Block) string,
) dcl.Diagnostics {
	var diags dcl.Diagnostics
	for i := range blocks {
		block := blocks[i]
		schema, ok := schemas[block.Type]
		if !ok || schema.Fields == nil {
			continue
		}
		version := versionFor(block)
		if version == "" {
			continue
		}
		for _, attr := range block.Attributes {
			hint, present := schema.Fields[attr.Key]
			if !present {
				continue
			}
			if d := checkField(attr.Key, hint, version, attr.Rng); d != nil {
				diags = append(diags, *d)
			}
		}
		for j := range block.Blocks {
			nested := block.Blocks[j]
			hint, present := schema.Fields[nested.Type]
			if !present {
				continue
			}
			if d := checkField(nested.Type, hint, version, nested.Rng); d != nil {
				diags = append(diags, *d)
			}
		}
	}
	return diags
}

// checkField compares the declared version against a field's
// MinVersion/MaxVersion. Returns a diagnostic on violation, or nil.
func checkField(name string, hint provider.FieldHint, version string, rng dcl.Range) *dcl.Diagnostic {
	if hint.MinVersion != "" {
		cmp, err := provider.CompareVersions(version, hint.MinVersion)
		if err != nil {
			return &dcl.Diagnostic{
				Severity: dcl.SeverityError,
				Message:  fmt.Sprintf("version comparison for %q failed: %s", name, err),
				Range:    rng,
			}
		}
		if cmp < 0 {
			return &dcl.Diagnostic{
				Severity: dcl.SeverityError,
				Message:  fmt.Sprintf("%q requires version %s or later; this context targets %s", name, hint.MinVersion, version),
				Range:    rng,
			}
		}
	}
	if hint.MaxVersion != "" {
		cmp, err := provider.CompareVersions(version, hint.MaxVersion)
		if err != nil {
			return &dcl.Diagnostic{
				Severity: dcl.SeverityError,
				Message:  fmt.Sprintf("version comparison for %q failed: %s", name, err),
				Range:    rng,
			}
		}
		if cmp > 0 {
			return &dcl.Diagnostic{
				Severity: dcl.SeverityError,
				Message:  fmt.Sprintf("%q was removed after version %s; this context targets %s", name, hint.MaxVersion, version),
				Range:    rng,
			}
		}
	}
	return nil
}
