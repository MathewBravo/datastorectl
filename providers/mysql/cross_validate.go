package mysql

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// ValidateResources runs MySQL-specific cross-resource checks that a
// per-resource Validate cannot express. It catches two classes of
// misconfiguration that would otherwise surface at apply time as raw
// MySQL errors:
//
//  1. Two mysql_user blocks with different DCL labels but identical
//     (user, host) tuples — both declarations target the same server row.
//  2. A mysql_user and a mysql_role sharing the same effective identity
//     (role host is always "%"). MySQL 8 stores roles in mysql.user, so
//     the two DCL blocks would race each other for the same server row.
//
// Resources with missing user or host fields are skipped — per-resource
// Validate reports those with a clearer message.
//
// This is invoked by the engine after Normalize, when mysql_user resource
// IDs are already in canonical "user@host" form. The function ignores
// types other than mysql_user and mysql_role.
func (p *Provider) ValidateResources(_ context.Context, resources []provider.Resource) dcl.Diagnostics {
	byIdentity := make(map[string][]entry)
	var order []string

	for _, r := range resources {
		var user, host, kind string
		switch r.ID.Type {
		case "mysql_user":
			user = getBodyString(r.Body, "user")
			host = getBodyString(r.Body, "host")
			kind = "user"
		case "mysql_role":
			// Role host is always "%" (see role.go). The role name is
			// the block label, which Normalize does not rewrite.
			user = r.ID.Name
			host = roleHost
			kind = "role"
		default:
			continue
		}
		if user == "" || host == "" {
			continue
		}
		identity := user + "@" + host
		if _, seen := byIdentity[identity]; !seen {
			order = append(order, identity)
		}
		byIdentity[identity] = append(byIdentity[identity], entry{resource: r, kind: kind})
	}

	var diags dcl.Diagnostics
	for _, identity := range order {
		entries := byIdentity[identity]
		if len(entries) < 2 {
			continue
		}
		diags = append(diags, buildCollisionDiagnostic(identity, entries))
	}
	return diags
}

// buildCollisionDiagnostic composes a single error diagnostic describing
// a set of resources that collapse onto the same MySQL server identity.
// The diagnostic's Range points at the last occurrence so editors land
// the cursor there; the message enumerates every block and its source
// position so the operator can find both sides of the collision.
func buildCollisionDiagnostic(identity string, entries []entry) dcl.Diagnostic {
	sorted := make([]entry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		return compareRanges(sorted[i].resource.SourceRange, sorted[j].resource.SourceRange) < 0
	})

	user, host := splitIdentity(identity)
	kinds := distinctKinds(sorted)
	refs := make([]string, len(sorted))
	for i, e := range sorted {
		refs[i] = fmt.Sprintf("%s.%s (%s)", e.resource.ID.Type, e.resource.ID.Name, e.resource.SourceRange.Start)
	}
	joined := strings.Join(refs, ", ")

	var message, suggestion string
	switch {
	case len(kinds) == 1 && kinds[0] == "user":
		message = fmt.Sprintf(
			"duplicate mysql_user identity user=%q host=%q declared by %s; these blocks all resolve to the same server row",
			user, host, joined,
		)
		suggestion = "change the user or host on one of these blocks so each mysql_user maps to a distinct (user, host) tuple"
	case len(kinds) == 1 && kinds[0] == "role":
		// Shouldn't happen — the engine rejects two blocks with the same
		// ResourceID at convert time — but guard defensively.
		message = fmt.Sprintf(
			"duplicate mysql_role identity name=%q declared by %s",
			user, joined,
		)
		suggestion = "rename one of the mysql_role blocks so each role name is unique"
	default:
		message = fmt.Sprintf(
			"mysql_user and mysql_role collide on the same MySQL 8 server row (user=%q host=%q): %s; MySQL 8 stores roles and users in the same mysql.user table, so both declarations would target the same row",
			user, host, joined,
		)
		suggestion = "rename the role so it does not collide with the user, or rename the user"
	}

	return dcl.Diagnostic{
		Severity:   dcl.SeverityError,
		Message:    message,
		Range:      sorted[len(sorted)-1].resource.SourceRange,
		Suggestion: suggestion,
	}
}

// compareRanges orders two ranges by filename, line, then column so
// diagnostics enumerate conflicting blocks in source order.
func compareRanges(a, b dcl.Range) int {
	if a.Start.Filename != b.Start.Filename {
		if a.Start.Filename < b.Start.Filename {
			return -1
		}
		return 1
	}
	if a.Start.Line != b.Start.Line {
		if a.Start.Line < b.Start.Line {
			return -1
		}
		return 1
	}
	if a.Start.Column != b.Start.Column {
		if a.Start.Column < b.Start.Column {
			return -1
		}
		return 1
	}
	return 0
}

// splitIdentity decomposes a "user@host" key back into its components.
// The last "@" is the separator; MySQL usernames may not contain "@"
// but we prefer right-split defensively.
func splitIdentity(identity string) (string, string) {
	i := strings.LastIndex(identity, "@")
	if i < 0 {
		return identity, ""
	}
	return identity[:i], identity[i+1:]
}

// distinctKinds returns the unique kinds ("user", "role") present in the
// entries, order-preserving on first occurrence.
func distinctKinds(entries []entry) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		if seen[e.kind] {
			continue
		}
		seen[e.kind] = true
		out = append(out, e.kind)
	}
	return out
}

// entry pairs a colliding resource with the kind of DCL block it came
// from so buildCollisionDiagnostic can pick the right message.
type entry struct {
	resource provider.Resource
	kind     string
}
