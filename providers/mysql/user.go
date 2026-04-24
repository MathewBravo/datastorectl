package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/MathewBravo/datastorectl/provider"
	"github.com/MathewBravo/datastorectl/providers/mysql/auth"
	"github.com/MathewBravo/datastorectl/providers/mysql/parse"
)

// userHandler manages mysql_user resources. A user is identified by
// the (user, host) tuple. The DCL block label is a free-form handle;
// Normalize rewrites ID.Name to "user@host" so the engine's by-ID
// diff pairs declared and discovered resources correctly.
type userHandler struct {
	// version is set by the provider after Configure. Currently unused
	// by user.go but plumbed for future version-gated parsing.
	version string
}

// Validate covers per-resource rules that don't need a cluster:
// required fields present, password shape valid, plugin supported.
func (h *userHandler) Validate(_ context.Context, r provider.Resource) error {
	user := getBodyString(r.Body, "user")
	host := getBodyString(r.Body, "host")
	if user == "" {
		return fmt.Errorf("mysql_user %q: required attribute user is missing or empty", r.ID.Name)
	}
	if host == "" {
		return fmt.Errorf("mysql_user %q: required attribute host is missing or empty", r.ID.Name)
	}
	if strings.ContainsAny(user, "`") || strings.ContainsAny(host, "`") {
		return fmt.Errorf("mysql_user %q: user/host contains a backtick, which is not a valid identifier character", r.ID.Name)
	}
	plugin := getBodyString(r.Body, "auth_plugin")
	if plugin == "" {
		plugin = auth.PluginCachingSHA2
	}
	decl := auth.Declared{
		Plugin:    plugin,
		Cleartext: getBodyString(r.Body, "password"),
		Hash:      getBodyString(r.Body, "password_hash"),
	}
	if err := auth.ValidateDeclared(decl); err != nil {
		return fmt.Errorf("mysql_user %q: %w", r.ID.Name, err)
	}
	return nil
}

// Normalize rewrites ID.Name to the canonical "user@host" form.
// Called on both declared (from DCL) and discovered resources, so
// the engine's diff sees identical identities for the same server row.
func (h *userHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	user := getBodyString(r.Body, "user")
	host := getBodyString(r.Body, "host")
	if user != "" && host != "" {
		r.ID.Name = user + "@" + host
	}
	return r, nil
}

// Equal answers "do these two users match?" with plugin-aware logic
// that the engine's structural diff cannot express. Specifically, it
// delegates password comparison to auth.Compare (which knows how to
// rehash declared cleartext against the live server's stored salt),
// then structurally compares the remaining attributes with default
// values filled in for anything the declared side omitted.
//
// Returns true only when password and every other non-defaulted
// attribute match. Returning false falls through to the engine's
// structural DiffResources so the plan still renders attribute-level
// output for ChangeUpdate.
func (h *userHandler) Equal(_ context.Context, desired, live provider.Resource) (bool, error) {
	plugin := getBodyString(desired.Body, "auth_plugin")
	if plugin == "" {
		plugin = auth.PluginCachingSHA2
	}
	decl := auth.Declared{
		Plugin:    plugin,
		Cleartext: getBodyString(desired.Body, "password"),
		Hash:      getBodyString(desired.Body, "password_hash"),
	}
	liveHash := getBodyString(live.Body, "password_hash")
	livePlugin := getBodyString(live.Body, "auth_plugin")

	// Plugin shape check first. aws_iam is compared by plugin name
	// alone (the server plugin is AWSAuthenticationPlugin, which is
	// what discovered reports).
	switch plugin {
	case auth.PluginAWSIAM:
		if livePlugin != "AWSAuthenticationPlugin" && livePlugin != auth.PluginAWSIAM {
			return false, nil
		}
	case auth.PluginCachingSHA2, auth.PluginNativePassword:
		if livePlugin != "" && livePlugin != plugin {
			return false, nil
		}
		match, err := auth.Compare(decl, liveHash)
		if err != nil {
			return false, fmt.Errorf("mysql_user %q: %w", desired.ID.Name, err)
		}
		if !match {
			return false, nil
		}
	default:
		return false, nil
	}

	// All other attributes compared structurally. Defaults filled in
	// for anything the declared side omitted so declared "account_locked
	// not set" matches discovered "account_locked = false".
	return userAttrsEqual(desired, live), nil
}

// userAttrsEqual compares every non-password mysql_user attribute,
// treating unset declared attributes as their default value. Password
// comparison is already handled by auth.Compare in the caller.
func userAttrsEqual(desired, live provider.Resource) bool {
	if getBodyBool(desired.Body, "account_locked") != getBodyBool(live.Body, "account_locked") {
		return false
	}
	for _, key := range []string{
		"require_ssl", "require_x509",
	} {
		if getBodyBool(desired.Body, key) != getBodyBool(live.Body, key) {
			return false
		}
	}
	for _, key := range []string{
		"require_issuer", "require_subject", "require_cipher",
		"comment", "attribute",
	} {
		if getBodyString(desired.Body, key) != getBodyString(live.Body, key) {
			return false
		}
	}
	for _, key := range []string{
		"max_queries_per_hour", "max_connections_per_hour",
		"max_updates_per_hour", "max_user_connections",
		"password_expire_days", "password_history",
		"password_reuse_interval",
	} {
		if getBodyInt(desired.Body, key) != getBodyInt(live.Body, key) {
			return false
		}
	}
	return true
}

// Discover enumerates every user on the server that isn't a role
// (i.e. account_locked='N' OR authentication_string<>''). Per user it
// runs SHOW CREATE USER and parses the output via the Phase 21 parser,
// then maps CreateUserStmt fields onto body attributes.
func (h *userHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	rows, err := client.DB().QueryContext(ctx, `
		SELECT User, Host FROM mysql.user
		WHERE NOT (account_locked = 'Y' AND authentication_string = '')
	`)
	if err != nil {
		return nil, fmt.Errorf("mysql_user discover: %w", err)
	}
	type userRow struct{ user, host string }
	var rowsList []userRow
	for rows.Next() {
		var u, hst string
		if err := rows.Scan(&u, &hst); err != nil {
			rows.Close()
			return nil, fmt.Errorf("mysql_user discover scan: %w", err)
		}
		rowsList = append(rowsList, userRow{u, hst})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]provider.Resource, 0, len(rowsList))
	for _, row := range rowsList {
		ddl, err := h.fetchShowCreateUser(ctx, client.DB(), row.user, row.host)
		if err != nil {
			return nil, err
		}
		stmt, err := parse.ParseCreateUser(ddl, h.version)
		if err != nil {
			return nil, fmt.Errorf("mysql_user %q@%q: parse SHOW CREATE USER: %w", row.user, row.host, err)
		}
		out = append(out, createUserStmtToResource(stmt))
	}
	return out, nil
}

func (h *userHandler) fetchShowCreateUser(ctx context.Context, db *sql.DB, user, host string) (string, error) {
	stmt := fmt.Sprintf("SHOW CREATE USER `%s`@`%s`", escapeBacktick(user), escapeBacktick(host))
	row := db.QueryRowContext(ctx, stmt)
	var ddl string
	if err := row.Scan(&ddl); err != nil {
		return "", fmt.Errorf("SHOW CREATE USER `%s`@`%s`: %w", user, host, err)
	}
	return ddl, nil
}

// createUserStmtToResource maps the parser output onto a Resource
// whose Body attributes match the mysql_user DCL schema.
func createUserStmtToResource(s parse.CreateUserStmt) provider.Resource {
	body := provider.NewOrderedMap()
	body.Set("user", provider.StringVal(s.User))
	body.Set("host", provider.StringVal(s.Host))
	body.Set("auth_plugin", provider.StringVal(s.Plugin))
	if s.AuthString != "" {
		body.Set("password_hash", provider.StringVal(s.AuthString))
	}
	if s.RequireSSL {
		body.Set("require_ssl", provider.BoolVal(true))
	}
	if s.RequireX509 {
		body.Set("require_x509", provider.BoolVal(true))
	}
	if s.RequireIssuer != "" {
		body.Set("require_issuer", provider.StringVal(s.RequireIssuer))
	}
	if s.RequireSubject != "" {
		body.Set("require_subject", provider.StringVal(s.RequireSubject))
	}
	if s.RequireCipher != "" {
		body.Set("require_cipher", provider.StringVal(s.RequireCipher))
	}
	if s.MaxQueriesPerHour != 0 {
		body.Set("max_queries_per_hour", provider.IntVal(int64(s.MaxQueriesPerHour)))
	}
	if s.MaxConnectionsPerHour != 0 {
		body.Set("max_connections_per_hour", provider.IntVal(int64(s.MaxConnectionsPerHour)))
	}
	if s.MaxUpdatesPerHour != 0 {
		body.Set("max_updates_per_hour", provider.IntVal(int64(s.MaxUpdatesPerHour)))
	}
	if s.MaxUserConnections != 0 {
		body.Set("max_user_connections", provider.IntVal(int64(s.MaxUserConnections)))
	}
	if s.PasswordExpire == "INTERVAL" {
		body.Set("password_expire_days", provider.IntVal(int64(s.PasswordExpireInterval)))
	}
	if s.PasswordHistory == "N" {
		body.Set("password_history", provider.IntVal(int64(s.PasswordHistoryCount)))
	}
	if s.PasswordReuse == "INTERVAL" {
		body.Set("password_reuse_interval", provider.IntVal(int64(s.PasswordReuseInterval)))
	}
	body.Set("account_locked", provider.BoolVal(s.AccountLocked))
	if s.Comment != "" {
		body.Set("comment", provider.StringVal(s.Comment))
	}
	if s.Attribute != "" {
		body.Set("attribute", provider.StringVal(s.Attribute))
	}
	return provider.Resource{
		ID:   provider.ResourceID{Type: "mysql_user", Name: s.User + "@" + s.Host},
		Body: body,
	}
}

// Apply dispatches create/update/delete to the appropriate builder.
func (h *userHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate:
		return h.create(ctx, client.DB(), r)
	case provider.OpUpdate:
		return h.update(ctx, client.DB(), r)
	case provider.OpDelete:
		return h.delete(ctx, client.DB(), r)
	}
	return fmt.Errorf("mysql_user: unsupported operation %s", op)
}

func (h *userHandler) create(ctx context.Context, db *sql.DB, r provider.Resource) error {
	stmt := buildUserStatement("CREATE USER", r)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql_user create %q: %w", r.ID.Name, err)
	}
	return nil
}

func (h *userHandler) update(ctx context.Context, db *sql.DB, r provider.Resource) error {
	stmt := buildUserStatement("ALTER USER", r)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql_user update %q: %w", r.ID.Name, err)
	}
	return nil
}

func (h *userHandler) delete(ctx context.Context, db *sql.DB, r provider.Resource) error {
	user := getBodyString(r.Body, "user")
	host := getBodyString(r.Body, "host")
	stmt := fmt.Sprintf("DROP USER `%s`@`%s`", escapeBacktick(user), escapeBacktick(host))
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("mysql_user delete %q: %w", r.ID.Name, err)
	}
	return nil
}

// buildUserStatement composes a CREATE USER or ALTER USER statement
// with every declared clause. On update we re-emit every clause (no
// surgical clause-by-clause diff); MySQL accepts idempotent re-issues
// of unchanged values, so this stays simple and correct.
func buildUserStatement(verb string, r provider.Resource) string {
	user := getBodyString(r.Body, "user")
	host := getBodyString(r.Body, "host")
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s `%s`@`%s`", verb, escapeBacktick(user), escapeBacktick(host))

	plugin := getBodyString(r.Body, "auth_plugin")
	if plugin == "" {
		plugin = auth.PluginCachingSHA2
	}
	cleartext := getBodyString(r.Body, "password")
	hash := getBodyString(r.Body, "password_hash")

	switch plugin {
	case auth.PluginAWSIAM:
		sb.WriteString(" IDENTIFIED WITH 'AWSAuthenticationPlugin' AS 'RDS'")
	case auth.PluginNativePassword, auth.PluginCachingSHA2:
		fmt.Fprintf(&sb, " IDENTIFIED WITH '%s'", plugin)
		switch {
		case hash != "":
			fmt.Fprintf(&sb, " AS '%s'", sqlEscapeSingle(hash))
		case cleartext != "":
			fmt.Fprintf(&sb, " BY '%s'", sqlEscapeSingle(cleartext))
		}
	}

	if s := getBodyString(r.Body, "require_subject"); s != "" {
		fmt.Fprintf(&sb, " REQUIRE SUBJECT '%s'", sqlEscapeSingle(s))
		if iss := getBodyString(r.Body, "require_issuer"); iss != "" {
			fmt.Fprintf(&sb, " AND ISSUER '%s'", sqlEscapeSingle(iss))
		}
		if ciph := getBodyString(r.Body, "require_cipher"); ciph != "" {
			fmt.Fprintf(&sb, " AND CIPHER '%s'", sqlEscapeSingle(ciph))
		}
	} else if getBodyBool(r.Body, "require_x509") {
		sb.WriteString(" REQUIRE X509")
	} else if getBodyBool(r.Body, "require_ssl") {
		sb.WriteString(" REQUIRE SSL")
	}

	wroteWith := false
	writeWith := func(label string, n int) {
		if n == 0 {
			return
		}
		if !wroteWith {
			sb.WriteString(" WITH")
			wroteWith = true
		}
		fmt.Fprintf(&sb, " %s %d", label, n)
	}
	writeWith("MAX_QUERIES_PER_HOUR", getBodyInt(r.Body, "max_queries_per_hour"))
	writeWith("MAX_CONNECTIONS_PER_HOUR", getBodyInt(r.Body, "max_connections_per_hour"))
	writeWith("MAX_UPDATES_PER_HOUR", getBodyInt(r.Body, "max_updates_per_hour"))
	writeWith("MAX_USER_CONNECTIONS", getBodyInt(r.Body, "max_user_connections"))

	if days := getBodyInt(r.Body, "password_expire_days"); days > 0 {
		fmt.Fprintf(&sb, " PASSWORD EXPIRE INTERVAL %d DAY", days)
	}
	if hist := getBodyInt(r.Body, "password_history"); hist > 0 {
		fmt.Fprintf(&sb, " PASSWORD HISTORY %d", hist)
	}
	if reuse := getBodyInt(r.Body, "password_reuse_interval"); reuse > 0 {
		fmt.Fprintf(&sb, " PASSWORD REUSE INTERVAL %d DAY", reuse)
	}

	if getBodyBool(r.Body, "account_locked") {
		sb.WriteString(" ACCOUNT LOCK")
	}

	if c := getBodyString(r.Body, "comment"); c != "" {
		fmt.Fprintf(&sb, " COMMENT '%s'", sqlEscapeSingle(c))
	}
	if a := getBodyString(r.Body, "attribute"); a != "" {
		fmt.Fprintf(&sb, " ATTRIBUTE '%s'", sqlEscapeSingle(a))
	}

	return sb.String()
}

// sqlEscapeSingle escapes a string to fit inside single-quoted SQL
// literals. Only single-quote doubling is strictly needed for the
// values we produce (passwords, hashes, subject/issuer DNs, comments,
// JSON attribute strings); backslash escaping matches what MySQL emits
// and keeps re-reading stable.
func sqlEscapeSingle(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "''")
	return s
}
