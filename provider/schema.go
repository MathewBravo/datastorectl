package provider

import (
	"fmt"
	"strconv"
	"strings"
)

// FieldKind tells the DCL converter how to represent a nested block
// group. The zero value means "no block hint" — used for scalar
// attributes that only carry version constraints.
type FieldKind int

const (
	// FieldBlockList means the block should always produce a ListVal,
	// even when only one block of that type appears.
	FieldBlockList FieldKind = iota + 1

	// FieldBlockMap means the block should always produce a MapVal,
	// even when multiple blocks of that type appear (which would be an error).
	FieldBlockMap
)

// FieldHint is the per-field schema annotation. Kind determines list
// vs map rendering for nested blocks; MinVersion and MaxVersion gate
// the field against the context's declared target version (ADR 0009).
// Empty version strings mean "no constraint on this end."
type FieldHint struct {
	Kind       FieldKind
	MinVersion string
	MaxVersion string
}

// Schema declares the expected structure for a resource type's fields.
// The converter uses Kind hints for list vs map; the engine uses
// version constraints for validate-time gating.
type Schema struct {
	Fields map[string]FieldHint
}

// CompareVersions compares two semantic-version strings by major.minor
// (patch and suffixes are ignored). Returns -1 / 0 / 1 in the usual
// sense. An empty string on either side is treated as "no constraint,"
// which yields 0.
func CompareVersions(a, b string) (int, error) {
	if a == "" || b == "" {
		return 0, nil
	}
	aMajor, aMinor, err := parseMajorMinor(a)
	if err != nil {
		return 0, fmt.Errorf("version %q: %w", a, err)
	}
	bMajor, bMinor, err := parseMajorMinor(b)
	if err != nil {
		return 0, fmt.Errorf("version %q: %w", b, err)
	}
	switch {
	case aMajor < bMajor:
		return -1, nil
	case aMajor > bMajor:
		return 1, nil
	case aMinor < bMinor:
		return -1, nil
	case aMinor > bMinor:
		return 1, nil
	default:
		return 0, nil
	}
}

// parseMajorMinor extracts the major and minor components from a
// version string. Trailing patch, pre-release, and build suffixes are
// tolerated and ignored.
func parseMajorMinor(v string) (major, minor int, err error) {
	// Strip suffix after any non-digit-or-dot character (e.g., "-rds").
	clean := v
	for i, r := range v {
		if r == '-' || r == '+' {
			clean = v[:i]
			break
		}
	}
	parts := strings.Split(clean, ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("must be major.minor (got %q)", v)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("major %q not a number", parts[0])
	}
	if major < 0 {
		return 0, 0, fmt.Errorf("major %d cannot be negative", major)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("minor %q not a number", parts[1])
	}
	if minor < 0 {
		return 0, 0, fmt.Errorf("minor %d cannot be negative", minor)
	}
	return major, minor, nil
}
