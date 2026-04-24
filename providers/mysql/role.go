package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/MathewBravo/datastorectl/provider"
)

// roleHandler manages mysql_role resources. MySQL 8 roles are stored
// as users with ACCOUNT LOCK and empty authentication_string. Discover
// filters mysql.user on those two columns; role-to-role grants live in
// mysql.role_edges.
//
// v0.1.0 simplification: role host is always "%". The DCL block label
// is the role name. If users need host-specific roles later, the body
// can grow a host attribute without breaking this handler.
type roleHandler struct{}

const roleHost = "%"

// Validate covers the per-resource rules that don't need a cluster.
// Cross-resource checks (e.g. role name colliding with a mysql_user
// in the same config) are a future engine-level concern.
func (h *roleHandler) Validate(_ context.Context, r provider.Resource) error {
	name := r.ID.Name
	if name == "" {
		return fmt.Errorf("mysql_role name cannot be empty")
	}
	if strings.ContainsAny(name, "`") {
		return fmt.Errorf("mysql_role name %q contains a backtick, which is not a valid identifier character", name)
	}
	return nil
}

// Normalize sorts the granted_roles list so the engine's structural
// diff sees a deterministic order regardless of how the user wrote
// the DCL or what order the server returned edges in.
func (h *roleHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	if r.Body == nil {
		return r, nil
	}
	v, ok := r.Body.Get("granted_roles")
	if !ok || v.Kind != provider.KindList {
		return r, nil
	}
	names := make([]string, 0, len(v.List))
	for _, e := range v.List {
		if e.Kind == provider.KindString {
			names = append(names, e.Str)
		}
	}
	sort.Strings(names)
	elems := make([]provider.Value, len(names))
	for i, n := range names {
		elems[i] = provider.StringVal(n)
	}
	r.Body.Set("granted_roles", provider.ListVal(elems))
	return r, nil
}

// Discover returns every MySQL 8 role — users with account_locked = 'Y'
// AND authentication_string = ''. For each, it joins mysql.role_edges
// to collect role-to-role grants (FROM_USER is the granted role,
// TO_USER is the recipient; we want edges where TO_USER is this role).
func (h *roleHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	rows, err := client.DB().QueryContext(ctx, `
		SELECT User FROM mysql.user
		WHERE account_locked = 'Y' AND authentication_string = ''
	`)
	if err != nil {
		return nil, fmt.Errorf("mysql_role discover: %w", err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("mysql_role discover scan: %w", err)
		}
		names = append(names, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql_role discover iterate: %w", err)
	}

	out := make([]provider.Resource, 0, len(names))
	for _, name := range names {
		granted, err := h.discoverGrantedRoles(ctx, client.DB(), name)
		if err != nil {
			return nil, err
		}
		body := provider.NewOrderedMap()
		elems := make([]provider.Value, len(granted))
		for i, g := range granted {
			elems[i] = provider.StringVal(g)
		}
		body.Set("granted_roles", provider.ListVal(elems))
		out = append(out, provider.Resource{
			ID:   provider.ResourceID{Type: "mysql_role", Name: name},
			Body: body,
		})
	}
	return out, nil
}

// discoverGrantedRoles reads the mysql.role_edges rows where the given
// user is the recipient. Returns the FROM_USER names sorted.
func (h *roleHandler) discoverGrantedRoles(ctx context.Context, db *sql.DB, recipient string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT FROM_USER FROM mysql.role_edges
		WHERE TO_USER = ? AND TO_HOST = ?
	`, recipient, roleHost)
	if err != nil {
		return nil, fmt.Errorf("mysql_role discover edges for %q: %w", recipient, err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("mysql_role discover edges scan: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// Apply dispatches by operation. Create and Update each reconcile the
// role's existence and its granted_roles. Delete drops the role —
// MySQL cascades role_edges rows automatically.
func (h *roleHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate:
		return h.create(ctx, client.DB(), r)
	case provider.OpUpdate:
		return h.update(ctx, client.DB(), r)
	case provider.OpDelete:
		return h.delete(ctx, client.DB(), r)
	}
	return fmt.Errorf("mysql_role: unsupported operation %s", op)
}

func (h *roleHandler) create(ctx context.Context, db *sql.DB, r provider.Resource) error {
	stmt := fmt.Sprintf("CREATE ROLE `%s`@`%s`", escapeBacktick(r.ID.Name), roleHost)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql_role create %q: %w", r.ID.Name, err)
	}
	declared := declaredGrantedRoles(r)
	for _, grant := range declared {
		if err := h.grantRole(ctx, db, grant, r.ID.Name); err != nil {
			return err
		}
	}
	return nil
}

// update reconciles granted_roles against the server. Since the role
// itself has no other mutable fields in v0.1.0, this is entirely a
// role_edges reconciliation: compute the set diff between declared and
// current, then issue GRANT / REVOKE accordingly.
func (h *roleHandler) update(ctx context.Context, db *sql.DB, r provider.Resource) error {
	current, err := h.discoverGrantedRoles(ctx, db, r.ID.Name)
	if err != nil {
		return err
	}
	declared := declaredGrantedRoles(r)

	declaredSet := toSet(declared)
	currentSet := toSet(current)

	// To add: declared \ current.
	for _, name := range declared {
		if !currentSet[name] {
			if err := h.grantRole(ctx, db, name, r.ID.Name); err != nil {
				return err
			}
		}
	}
	// To remove: current \ declared.
	for _, name := range current {
		if !declaredSet[name] {
			if err := h.revokeRole(ctx, db, name, r.ID.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *roleHandler) delete(ctx context.Context, db *sql.DB, r provider.Resource) error {
	stmt := fmt.Sprintf("DROP ROLE `%s`@`%s`", escapeBacktick(r.ID.Name), roleHost)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql_role delete %q: %w", r.ID.Name, err)
	}
	return nil
}

// grantRole runs `GRANT granted TO recipient` — makes recipient
// inherit granted's privileges when the role is activated.
func (h *roleHandler) grantRole(ctx context.Context, db *sql.DB, granted, recipient string) error {
	stmt := fmt.Sprintf("GRANT `%s`@`%s` TO `%s`@`%s`",
		escapeBacktick(granted), roleHost,
		escapeBacktick(recipient), roleHost)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql_role grant %q to %q: %w", granted, recipient, err)
	}
	return nil
}

// revokeRole runs `REVOKE granted FROM recipient`.
func (h *roleHandler) revokeRole(ctx context.Context, db *sql.DB, granted, recipient string) error {
	stmt := fmt.Sprintf("REVOKE `%s`@`%s` FROM `%s`@`%s`",
		escapeBacktick(granted), roleHost,
		escapeBacktick(recipient), roleHost)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql_role revoke %q from %q: %w", granted, recipient, err)
	}
	return nil
}

// declaredGrantedRoles extracts the granted_roles attribute from a
// resource body as a sorted string slice.
func declaredGrantedRoles(r provider.Resource) []string {
	if r.Body == nil {
		return nil
	}
	v, ok := r.Body.Get("granted_roles")
	if !ok || v.Kind != provider.KindList {
		return nil
	}
	out := make([]string, 0, len(v.List))
	for _, e := range v.List {
		if e.Kind == provider.KindString && e.Str != "" {
			out = append(out, e.Str)
		}
	}
	sort.Strings(out)
	return out
}

// toSet converts a slice to a set map for O(1) membership lookups.
func toSet(names []string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}
