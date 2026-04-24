package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/MathewBravo/datastorectl/provider"
	"github.com/MathewBravo/datastorectl/providers/mysql/parse"
)

// grantHandler manages mysql_grant resources. A grant is identified
// by the tuple (user, host, database, table). The DCL block label is
// a free-form handle; Normalize rewrites ID.Name to "user@host:db.tbl"
// so the engine's by-ID diff pairs declared and discovered resources.
type grantHandler struct {
	version string
}

// Validate covers per-resource rules. User/role identity collisions
// across blocks are handled by the provider's ValidateResources hook
// (see cross_validate.go); grant-specific cross-resource checks (e.g.
// a grant referencing a user/role the config does not declare) are not
// yet implemented.
func (h *grantHandler) Validate(_ context.Context, r provider.Resource) error {
	user := getBodyString(r.Body, "user")
	host := getBodyString(r.Body, "host")
	database := getBodyString(r.Body, "database")
	table := getBodyString(r.Body, "table")
	privs := getStringListField(r.Body, "privileges")

	if user == "" {
		return fmt.Errorf("mysql_grant %q: required attribute user is missing or empty", r.ID.Name)
	}
	if host == "" {
		return fmt.Errorf("mysql_grant %q: required attribute host is missing or empty", r.ID.Name)
	}
	if database == "" {
		return fmt.Errorf("mysql_grant %q: required attribute database is missing or empty (use \"*\" for global scope)", r.ID.Name)
	}
	if table == "" {
		return fmt.Errorf("mysql_grant %q: required attribute table is missing or empty (use \"*\" for schema-level scope)", r.ID.Name)
	}
	if len(privs) == 0 {
		return fmt.Errorf("mysql_grant %q: privileges list is missing or empty", r.ID.Name)
	}
	for i, p := range privs {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("mysql_grant %q: privileges[%d] is an empty privilege name", r.ID.Name, i)
		}
	}
	if strings.ContainsAny(user, "`") || strings.ContainsAny(host, "`") ||
		strings.ContainsAny(database, "`") || strings.ContainsAny(table, "`") {
		return fmt.Errorf("mysql_grant %q: identifier contains a backtick", r.ID.Name)
	}
	return nil
}

// Normalize rewrites ID.Name to the canonical identity tuple form and
// sorts + uppercases the privileges list so the engine's diff is
// deterministic.
func (h *grantHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	if r.Body == nil {
		return r, nil
	}
	user := getBodyString(r.Body, "user")
	host := getBodyString(r.Body, "host")
	database := getBodyString(r.Body, "database")
	table := getBodyString(r.Body, "table")
	if user != "" && host != "" && database != "" && table != "" {
		r.ID.Name = fmt.Sprintf("%s@%s:%s.%s", user, host, database, table)
	}

	privs := getStringListField(r.Body, "privileges")
	normalized := normalizePrivs(privs)
	elems := make([]provider.Value, len(normalized))
	for i, p := range normalized {
		elems[i] = provider.StringVal(p)
	}
	r.Body.Set("privileges", provider.ListVal(elems))
	return r, nil
}

// normalizePrivs uppercases, trims, dedupes, sorts, and collapses
// "ALL PRIVILEGES" to "ALL". Empty strings are dropped silently
// (Validate rejects them upstream).
func normalizePrivs(privs []string) []string {
	seen := make(map[string]bool, len(privs))
	out := make([]string, 0, len(privs))
	for _, p := range privs {
		up := strings.ToUpper(strings.TrimSpace(p))
		if up == "" {
			continue
		}
		if up == "ALL PRIVILEGES" {
			up = "ALL"
		}
		if !seen[up] {
			seen[up] = true
			out = append(out, up)
		}
	}
	sort.Strings(out)
	return out
}

// Discover enumerates grants across every non-role account on the
// server. For each, it runs SHOW GRANTS and parses every emitted GRANT
// statement into a separate resource. The USAGE-only placeholder grant
// every MySQL user gets by default is filtered out.
func (h *grantHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	rows, err := client.DB().QueryContext(ctx, `
		SELECT User, Host FROM mysql.user
		WHERE NOT (account_locked = 'Y' AND authentication_string = '')
	`)
	if err != nil {
		return nil, fmt.Errorf("mysql_grant discover users: %w", err)
	}
	type acct struct{ user, host string }
	var accounts []acct
	for rows.Next() {
		var u, hst string
		if err := rows.Scan(&u, &hst); err != nil {
			rows.Close()
			return nil, fmt.Errorf("mysql_grant discover scan: %w", err)
		}
		accounts = append(accounts, acct{u, hst})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Roles, too — privileges can be granted to roles.
	roleRows, err := client.DB().QueryContext(ctx, `
		SELECT User, Host FROM mysql.user
		WHERE account_locked = 'Y' AND authentication_string = ''
	`)
	if err != nil {
		return nil, fmt.Errorf("mysql_grant discover roles: %w", err)
	}
	for roleRows.Next() {
		var u, hst string
		if err := roleRows.Scan(&u, &hst); err != nil {
			roleRows.Close()
			return nil, err
		}
		accounts = append(accounts, acct{u, hst})
	}
	roleRows.Close()

	var out []provider.Resource
	for _, a := range accounts {
		stmts, err := h.fetchAndParseGrants(ctx, client.DB(), a.user, a.host)
		if err != nil {
			return nil, err
		}
		for _, s := range stmts {
			if isUsageOnlyDefault(s) {
				continue
			}
			out = append(out, grantStmtToResource(s))
		}
	}
	return out, nil
}

// isUsageOnlyDefault reports whether a parsed grant is the default
// USAGE placeholder MySQL attaches to every fresh account. We filter
// these because they're not user-managed state.
func isUsageOnlyDefault(s parse.GrantStmt) bool {
	if s.Database != "*" || s.Table != "*" || s.GrantOption {
		return false
	}
	if len(s.Privileges) != 1 {
		return false
	}
	return s.Privileges[0] == "USAGE"
}

func (h *grantHandler) fetchAndParseGrants(ctx context.Context, db *sql.DB, user, host string) ([]parse.GrantStmt, error) {
	stmt := fmt.Sprintf("SHOW GRANTS FOR `%s`@`%s`", escapeBacktick(user), escapeBacktick(host))
	rows, err := db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("SHOW GRANTS FOR `%s`@`%s`: %w", user, host, err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]parse.GrantStmt, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// GRANT PROXY grants use an empty-ident scope (``@``) that
		// the table-scope parser can't parse. Skip them as
		// out-of-scope for v0.1.0.
		if strings.HasPrefix(strings.ToUpper(trimmed), "GRANT PROXY ") {
			continue
		}
		// Skip column-level and routine grants the parser rejects.
		// Surface all other parse errors.
		s, err := parse.ParseGrant(trimmed, h.version)
		if err != nil {
			if strings.Contains(err.Error(), "not supported") {
				continue
			}
			return nil, fmt.Errorf("parse grant line %q: %w", line, err)
		}
		// Role-to-user grants (GRANT `role` TO `user`) share the SHOW
		// GRANTS output with privilege grants but don't map onto
		// mysql_grant. Skip them; a future mysql_role_grant resource
		// can consume the parsed form.
		if s.IsRoleGrant() {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// grantStmtToResource maps a parsed GRANT into a provider.Resource.
func grantStmtToResource(s parse.GrantStmt) provider.Resource {
	body := provider.NewOrderedMap()
	body.Set("user", provider.StringVal(s.User))
	body.Set("host", provider.StringVal(s.Host))
	body.Set("database", provider.StringVal(s.Database))
	body.Set("table", provider.StringVal(s.Table))
	elems := make([]provider.Value, len(s.Privileges))
	for i, p := range s.Privileges {
		elems[i] = provider.StringVal(p)
	}
	body.Set("privileges", provider.ListVal(elems))
	if s.GrantOption {
		body.Set("grant_option", provider.BoolVal(true))
	}
	return provider.Resource{
		ID:   provider.ResourceID{Type: "mysql_grant", Name: fmt.Sprintf("%s@%s:%s.%s", s.User, s.Host, s.Database, s.Table)},
		Body: body,
	}
}

// Apply runs the declared GRANT / REVOKE operations against the server.
func (h *grantHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate:
		return h.create(ctx, client.DB(), r)
	case provider.OpUpdate:
		return h.update(ctx, client.DB(), r)
	case provider.OpDelete:
		return h.delete(ctx, client.DB(), r)
	}
	return fmt.Errorf("mysql_grant: unsupported operation %s", op)
}

func (h *grantHandler) create(ctx context.Context, db *sql.DB, r provider.Resource) error {
	stmt := buildGrantStatement(r)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql_grant create %q: %w", r.ID.Name, err)
	}
	return nil
}

// update reconciles declared privileges against current server state
// by fetching SHOW GRANTS for the (user, host) and emitting only the
// needed GRANT and REVOKE statements for the target scope. This avoids
// re-granting everything on every apply.
func (h *grantHandler) update(ctx context.Context, db *sql.DB, r provider.Resource) error {
	user := getBodyString(r.Body, "user")
	host := getBodyString(r.Body, "host")
	database := getBodyString(r.Body, "database")
	table := getBodyString(r.Body, "table")

	declared := normalizePrivs(getStringListField(r.Body, "privileges"))
	declaredSet := toSet(declared)
	declaredGrantOpt := getBodyBool(r.Body, "grant_option")

	current, currentGrantOpt, err := h.currentPrivilegesForScope(ctx, db, user, host, database, table)
	if err != nil {
		return err
	}
	currentSet := toSet(current)

	// REVOKE what's no longer declared.
	var toRevoke []string
	for _, p := range current {
		if !declaredSet[p] {
			toRevoke = append(toRevoke, p)
		}
	}
	if len(toRevoke) > 0 {
		stmt := fmt.Sprintf("REVOKE %s ON %s FROM `%s`@`%s`",
			strings.Join(toRevoke, ", "),
			scopeFragment(database, table),
			escapeBacktick(user), escapeBacktick(host))
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("mysql_grant update revoke %q: %w", r.ID.Name, err)
		}
	}

	// GRANT what's newly declared.
	var toGrant []string
	for _, p := range declared {
		if !currentSet[p] {
			toGrant = append(toGrant, p)
		}
	}
	if len(toGrant) > 0 || declaredGrantOpt != currentGrantOpt {
		privList := toGrant
		if len(privList) == 0 {
			// Privileges unchanged but grant_option flipped — re-grant
			// the existing set with the new option setting.
			privList = declared
		}
		stmt := fmt.Sprintf("GRANT %s ON %s TO `%s`@`%s`",
			strings.Join(privList, ", "),
			scopeFragment(database, table),
			escapeBacktick(user), escapeBacktick(host))
		if declaredGrantOpt {
			stmt += " WITH GRANT OPTION"
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("mysql_grant update grant %q: %w", r.ID.Name, err)
		}
	}
	return nil
}

// currentPrivilegesForScope returns the privilege list + grant_option
// currently on the server for a specific (user, host, database, table)
// tuple. Returns empty list if no grant exists for that scope.
func (h *grantHandler) currentPrivilegesForScope(ctx context.Context, db *sql.DB, user, host, database, table string) ([]string, bool, error) {
	stmts, err := (&grantHandler{}).fetchAndParseGrants(ctx, db, user, host)
	if err != nil {
		return nil, false, err
	}
	for _, s := range stmts {
		if s.Database == database && s.Table == table {
			return normalizePrivs(s.Privileges), s.GrantOption, nil
		}
	}
	return nil, false, nil
}

func (h *grantHandler) delete(ctx context.Context, db *sql.DB, r provider.Resource) error {
	user := getBodyString(r.Body, "user")
	host := getBodyString(r.Body, "host")
	database := getBodyString(r.Body, "database")
	table := getBodyString(r.Body, "table")

	// Scope-specific revoke: MySQL accepts ALL PRIVILEGES here and
	// implicitly revokes the grant option for that scope. The global
	// form `REVOKE ALL PRIVILEGES, GRANT OPTION FROM user` has no ON
	// clause and applies across all scopes — not what we want.
	stmt := fmt.Sprintf("REVOKE ALL PRIVILEGES ON %s FROM `%s`@`%s`",
		scopeFragment(database, table),
		escapeBacktick(user), escapeBacktick(host))
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql_grant delete %q: %w", r.ID.Name, err)
	}
	return nil
}

// buildGrantStatement composes a full GRANT DDL for create.
func buildGrantStatement(r provider.Resource) string {
	user := getBodyString(r.Body, "user")
	host := getBodyString(r.Body, "host")
	database := getBodyString(r.Body, "database")
	table := getBodyString(r.Body, "table")
	privs := normalizePrivs(getStringListField(r.Body, "privileges"))
	grantOpt := getBodyBool(r.Body, "grant_option")

	var sb strings.Builder
	fmt.Fprintf(&sb, "GRANT %s ON %s TO `%s`@`%s`",
		strings.Join(privs, ", "),
		scopeFragment(database, table),
		escapeBacktick(user), escapeBacktick(host))
	if grantOpt {
		sb.WriteString(" WITH GRANT OPTION")
	}
	return sb.String()
}

// scopeFragment builds the `ON db.tbl` portion. Literal `*` means any.
func scopeFragment(database, table string) string {
	dbPart := "*"
	if database != "*" {
		dbPart = "`" + escapeBacktick(database) + "`"
	}
	tblPart := "*"
	if table != "*" {
		tblPart = "`" + escapeBacktick(table) + "`"
	}
	return dbPart + "." + tblPart
}
