package parse

import (
	"strings"
	"testing"
)

// Backtick helper for embedding ` in test DDL.
const bt = "`"

// TestParseCreateUser_BasicUser covers the simplest shape emitted by
// SHOW CREATE USER for a freshly created user with only the defaults.
func TestParseCreateUser_BasicUser(t *testing.T) {
	ddl := `CREATE USER ` + bt + `u_basic` + bt + `@` + bt + `%` + bt + ` IDENTIFIED WITH 'caching_sha2_password' AS '$A$005$salt20bytes-stored-hash-43chars' REQUIRE NONE PASSWORD EXPIRE DEFAULT ACCOUNT UNLOCK PASSWORD HISTORY DEFAULT PASSWORD REUSE INTERVAL DEFAULT PASSWORD REQUIRE CURRENT DEFAULT`

	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.User != "u_basic" {
		t.Errorf("User = %q, want %q", stmt.User, "u_basic")
	}
	if stmt.Host != "%" {
		t.Errorf("Host = %q, want %q", stmt.Host, "%")
	}
	if stmt.Plugin != "caching_sha2_password" {
		t.Errorf("Plugin = %q", stmt.Plugin)
	}
	if !strings.HasPrefix(stmt.AuthString, "$A$005$") {
		t.Errorf("AuthString prefix = %q, want $A$005$...", stmt.AuthString[:7])
	}
	if !stmt.RequireNone {
		t.Error("RequireNone = false, want true")
	}
	if stmt.PasswordExpire != "DEFAULT" {
		t.Errorf("PasswordExpire = %q, want DEFAULT", stmt.PasswordExpire)
	}
	if stmt.AccountLocked {
		t.Error("AccountLocked = true, want false (UNLOCK)")
	}
	if stmt.PasswordHistory != "DEFAULT" {
		t.Errorf("PasswordHistory = %q, want DEFAULT", stmt.PasswordHistory)
	}
	if stmt.PasswordReuse != "DEFAULT" {
		t.Errorf("PasswordReuse = %q, want DEFAULT", stmt.PasswordReuse)
	}
	if stmt.PasswordRequireCurrent != "DEFAULT" {
		t.Errorf("PasswordRequireCurrent = %q, want DEFAULT", stmt.PasswordRequireCurrent)
	}
}

// TestParseCreateUser_TLSFields covers REQUIRE SUBJECT/ISSUER/CIPHER
// as emitted by SHOW CREATE USER (space-separated, no AND keyword).
// This exact string was captured from a live mysql:8.4 container.
func TestParseCreateUser_TLSFields(t *testing.T) {
	ddl := `CREATE USER ` + bt + `u_tls` + bt + `@` + bt + `%` + bt +
		` IDENTIFIED WITH 'caching_sha2_password' AS '$A$005$somehash'` +
		` REQUIRE SUBJECT '/CN=client' ISSUER '/CN=Internal CA' CIPHER 'ECDHE-RSA-AES256-GCM-SHA384'` +
		` PASSWORD EXPIRE DEFAULT ACCOUNT UNLOCK PASSWORD HISTORY DEFAULT` +
		` PASSWORD REUSE INTERVAL DEFAULT PASSWORD REQUIRE CURRENT DEFAULT`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.RequireSubject != "/CN=client" {
		t.Errorf("RequireSubject = %q", stmt.RequireSubject)
	}
	if stmt.RequireIssuer != "/CN=Internal CA" {
		t.Errorf("RequireIssuer = %q", stmt.RequireIssuer)
	}
	if stmt.RequireCipher != "ECDHE-RSA-AES256-GCM-SHA384" {
		t.Errorf("RequireCipher = %q", stmt.RequireCipher)
	}
	if stmt.RequireNone {
		t.Error("RequireNone = true, want false")
	}
}

// TestParseCreateUser_RequireSSL covers the flag-only REQUIRE forms.
func TestParseCreateUser_RequireSSL(t *testing.T) {
	ddl := `CREATE USER ` + bt + `u` + bt + `@` + bt + `%` + bt +
		` IDENTIFIED WITH 'caching_sha2_password' AS ''` +
		` REQUIRE SSL PASSWORD EXPIRE DEFAULT ACCOUNT UNLOCK` +
		` PASSWORD HISTORY DEFAULT PASSWORD REUSE INTERVAL DEFAULT` +
		` PASSWORD REQUIRE CURRENT DEFAULT`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !stmt.RequireSSL {
		t.Error("RequireSSL = false, want true")
	}
}

// TestParseCreateUser_ResourceLimits covers the WITH clause.
func TestParseCreateUser_ResourceLimits(t *testing.T) {
	ddl := `CREATE USER ` + bt + `u` + bt + `@` + bt + `%` + bt +
		` IDENTIFIED WITH 'caching_sha2_password' AS ''` +
		` REQUIRE NONE` +
		` WITH MAX_QUERIES_PER_HOUR 1000 MAX_CONNECTIONS_PER_HOUR 50` +
		` MAX_UPDATES_PER_HOUR 500 MAX_USER_CONNECTIONS 10` +
		` PASSWORD EXPIRE DEFAULT ACCOUNT UNLOCK` +
		` PASSWORD HISTORY DEFAULT PASSWORD REUSE INTERVAL DEFAULT` +
		` PASSWORD REQUIRE CURRENT DEFAULT`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.MaxQueriesPerHour != 1000 {
		t.Errorf("MaxQueriesPerHour = %d", stmt.MaxQueriesPerHour)
	}
	if stmt.MaxConnectionsPerHour != 50 {
		t.Errorf("MaxConnectionsPerHour = %d", stmt.MaxConnectionsPerHour)
	}
	if stmt.MaxUpdatesPerHour != 500 {
		t.Errorf("MaxUpdatesPerHour = %d", stmt.MaxUpdatesPerHour)
	}
	if stmt.MaxUserConnections != 10 {
		t.Errorf("MaxUserConnections = %d", stmt.MaxUserConnections)
	}
}

// TestParseCreateUser_AccountLock covers ACCOUNT LOCK.
func TestParseCreateUser_AccountLock(t *testing.T) {
	ddl := `CREATE USER ` + bt + `u_locked` + bt + `@` + bt + `%` + bt +
		` IDENTIFIED WITH 'caching_sha2_password' AS ''` +
		` REQUIRE NONE PASSWORD EXPIRE DEFAULT ACCOUNT LOCK` +
		` PASSWORD HISTORY DEFAULT PASSWORD REUSE INTERVAL DEFAULT` +
		` PASSWORD REQUIRE CURRENT DEFAULT`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !stmt.AccountLocked {
		t.Error("AccountLocked = false, want true")
	}
}

// TestParseCreateUser_PasswordPolicyCustom covers non-DEFAULT password
// policy values including INTERVAL forms and numeric history count.
func TestParseCreateUser_PasswordPolicyCustom(t *testing.T) {
	ddl := `CREATE USER ` + bt + `u` + bt + `@` + bt + `%` + bt +
		` IDENTIFIED WITH 'caching_sha2_password' AS ''` +
		` REQUIRE NONE` +
		` PASSWORD EXPIRE INTERVAL 90 DAY` +
		` ACCOUNT UNLOCK` +
		` PASSWORD HISTORY 5` +
		` PASSWORD REUSE INTERVAL 365 DAY` +
		` PASSWORD REQUIRE CURRENT OPTIONAL` +
		` FAILED_LOGIN_ATTEMPTS 3 PASSWORD_LOCK_TIME 1`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.PasswordExpire != "INTERVAL" || stmt.PasswordExpireInterval != 90 {
		t.Errorf("PasswordExpire = %q / interval = %d", stmt.PasswordExpire, stmt.PasswordExpireInterval)
	}
	if stmt.PasswordHistory != "N" || stmt.PasswordHistoryCount != 5 {
		t.Errorf("PasswordHistory = %q / count = %d", stmt.PasswordHistory, stmt.PasswordHistoryCount)
	}
	if stmt.PasswordReuse != "INTERVAL" || stmt.PasswordReuseInterval != 365 {
		t.Errorf("PasswordReuse = %q / interval = %d", stmt.PasswordReuse, stmt.PasswordReuseInterval)
	}
	if stmt.PasswordRequireCurrent != "OPTIONAL" {
		t.Errorf("PasswordRequireCurrent = %q", stmt.PasswordRequireCurrent)
	}
	if stmt.FailedLoginAttempts != 3 {
		t.Errorf("FailedLoginAttempts = %d", stmt.FailedLoginAttempts)
	}
	if stmt.PasswordLockTime != "1" {
		t.Errorf("PasswordLockTime = %q", stmt.PasswordLockTime)
	}
}

// TestParseCreateUser_PasswordLockTimeUnbounded covers the UNBOUNDED
// variant of PASSWORD_LOCK_TIME.
func TestParseCreateUser_PasswordLockTimeUnbounded(t *testing.T) {
	ddl := `CREATE USER ` + bt + `u` + bt + `@` + bt + `%` + bt +
		` IDENTIFIED WITH 'caching_sha2_password' AS ''` +
		` REQUIRE NONE PASSWORD EXPIRE DEFAULT ACCOUNT UNLOCK` +
		` PASSWORD HISTORY DEFAULT PASSWORD REUSE INTERVAL DEFAULT` +
		` PASSWORD REQUIRE CURRENT DEFAULT` +
		` FAILED_LOGIN_ATTEMPTS 5 PASSWORD_LOCK_TIME UNBOUNDED`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.PasswordLockTime != "UNBOUNDED" {
		t.Errorf("PasswordLockTime = %q", stmt.PasswordLockTime)
	}
}

// TestParseCreateUser_CommentAndAttribute covers the trailing metadata
// clauses.
func TestParseCreateUser_CommentAndAttribute(t *testing.T) {
	ddl := `CREATE USER ` + bt + `u` + bt + `@` + bt + `%` + bt +
		` IDENTIFIED WITH 'caching_sha2_password' AS ''` +
		` REQUIRE NONE PASSWORD EXPIRE DEFAULT ACCOUNT UNLOCK` +
		` PASSWORD HISTORY DEFAULT PASSWORD REUSE INTERVAL DEFAULT` +
		` PASSWORD REQUIRE CURRENT DEFAULT` +
		` COMMENT 'provisioned by datastorectl'` +
		` ATTRIBUTE '{"team":"dd"}'`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.Comment != "provisioned by datastorectl" {
		t.Errorf("Comment = %q", stmt.Comment)
	}
	if stmt.Attribute != `{"team":"dd"}` {
		t.Errorf("Attribute = %q", stmt.Attribute)
	}
}

// TestParseCreateUser_InputFormWithAND covers the input-form REQUIRE
// syntax that uses AND between fields. While SHOW CREATE USER doesn't
// emit AND, users writing raw CREATE USER in DCL may include it —
// the parser tolerates both shapes.
func TestParseCreateUser_InputFormWithAND(t *testing.T) {
	ddl := `CREATE USER 'u'@'%' IDENTIFIED WITH 'caching_sha2_password' AS '' REQUIRE SUBJECT '/CN=x' AND ISSUER '/CN=y'`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.RequireSubject != "/CN=x" || stmt.RequireIssuer != "/CN=y" {
		t.Errorf("Subject=%q Issuer=%q", stmt.RequireSubject, stmt.RequireIssuer)
	}
}

// TestParseCreateUser_RoleShape covers MySQL 8's treatment of roles as
// users: account locked, empty auth string. The parser produces the
// same structure as for a regular user; the handler classifies from
// the AccountLocked + empty AuthString combo.
func TestParseCreateUser_RoleShape(t *testing.T) {
	ddl := `CREATE USER ` + bt + `reader` + bt + `@` + bt + `%` + bt +
		` IDENTIFIED WITH 'caching_sha2_password' AS ''` +
		` REQUIRE NONE PASSWORD EXPIRE ACCOUNT LOCK` +
		` PASSWORD HISTORY DEFAULT PASSWORD REUSE INTERVAL DEFAULT` +
		` PASSWORD REQUIRE CURRENT DEFAULT`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !stmt.AccountLocked || stmt.AuthString != "" {
		t.Errorf("not role-shaped: locked=%v authstring=%q", stmt.AccountLocked, stmt.AuthString)
	}
}

// TestParseCreateUser_MalformedInputRejected verifies that clearly
// malformed input surfaces as an error rather than silently producing
// a partial parse.
func TestParseCreateUser_MalformedInputRejected(t *testing.T) {
	cases := []string{
		"",
		"not a create user statement",
		"CREATE USER", // missing user/host
		"CREATE USER 'u'",
		"CREATE USER 'u'@",
		`CREATE USER 'u'@'%' IDENTIFIED`,      // missing WITH
		`CREATE USER 'u'@'%' UNKNOWN_CLAUSE 5`, // unknown clause
	}
	for i, c := range cases {
		if _, err := ParseCreateUser(c, "8.4"); err == nil {
			t.Errorf("case %d (%q): expected error, got none", i, c)
		}
	}
}

// TestParseCreateUser_SQLEscapesInAuthString verifies that SQL escape
// sequences in the AS '...' clause decode correctly. Real
// authentication strings contain non-printable bytes that MySQL emits
// with escapes like \Z (0x1a).
func TestParseCreateUser_SQLEscapesInAuthString(t *testing.T) {
	ddl := `CREATE USER 'u'@'%' IDENTIFIED WITH 'caching_sha2_password' AS '\Zbyte\tafter\ntab'`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	expected := "\x1abyte\tafter\ntab"
	if stmt.AuthString != expected {
		t.Errorf("AuthString = %q, want %q", stmt.AuthString, expected)
	}
}

// TestParseCreateUser_UnquotedSingleQuoteIdentForms covers the
// single-quoted alternative (not backticked) that valid CREATE USER
// statements also accept.
func TestParseCreateUser_UnquotedSingleQuoteIdentForms(t *testing.T) {
	ddl := `CREATE USER 'app_user'@'10.0.%' IDENTIFIED WITH 'caching_sha2_password' AS '$A$005$x'`
	stmt, err := ParseCreateUser(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.User != "app_user" || stmt.Host != "10.0.%" {
		t.Errorf("User=%q Host=%q", stmt.User, stmt.Host)
	}
}

// TestParseCreateUser_DefaultRoleAurora exercises the exact DDL shape
// AWS RDS Aurora emits for its master `admin` user. The `DEFAULT ROLE
// <role>@<host>` clause appears mid-stream between ACCOUNT UNLOCK and
// PASSWORD HISTORY; the parser must consume it without disturbing the
// surrounding clauses. Surfaced by #201; tracked in #214.
func TestParseCreateUser_DefaultRoleAurora(t *testing.T) {
	ddl := `CREATE USER ` + bt + `admin` + bt + `@` + bt + `%` + bt +
		` IDENTIFIED WITH 'caching_sha2_password' AS '$A$005$somehash'` +
		` REQUIRE NONE PASSWORD EXPIRE DEFAULT ACCOUNT UNLOCK` +
		` DEFAULT ROLE ` + bt + `rds_superuser_role` + bt + `@` + bt + `%` + bt +
		` PASSWORD HISTORY DEFAULT PASSWORD REUSE INTERVAL DEFAULT` +
		` PASSWORD REQUIRE CURRENT DEFAULT`
	stmt, err := ParseCreateUser(ddl, "8.0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stmt.DefaultRoles) != 1 {
		t.Fatalf("DefaultRoles length = %d, want 1: %+v", len(stmt.DefaultRoles), stmt.DefaultRoles)
	}
	got := stmt.DefaultRoles[0]
	if got.Name != "rds_superuser_role" || got.Host != "%" {
		t.Errorf("DefaultRoles[0] = %+v, want {Name: rds_superuser_role, Host: %%}", got)
	}
	// Ensure the clause didn't swallow or confuse neighboring fields.
	if stmt.PasswordHistory != "DEFAULT" {
		t.Errorf("PasswordHistory = %q, want DEFAULT (clause after DEFAULT ROLE was skipped)", stmt.PasswordHistory)
	}
	if stmt.PasswordRequireCurrent != "DEFAULT" {
		t.Errorf("PasswordRequireCurrent = %q, want DEFAULT", stmt.PasswordRequireCurrent)
	}
}

// TestParseCreateUser_DefaultRoleMultiple covers the comma-separated
// multi-role form of DEFAULT ROLE. MySQL 8 accepts DEFAULT ROLE r1, r2,
// r3 and SHOW CREATE USER emits each with its host.
func TestParseCreateUser_DefaultRoleMultiple(t *testing.T) {
	ddl := `CREATE USER ` + bt + `u` + bt + `@` + bt + `%` + bt +
		` IDENTIFIED WITH 'caching_sha2_password' AS ''` +
		` REQUIRE NONE PASSWORD EXPIRE DEFAULT ACCOUNT UNLOCK` +
		` DEFAULT ROLE ` + bt + `reader` + bt + `@` + bt + `%` + bt +
		`, ` + bt + `writer` + bt + `@` + bt + `%` + bt +
		`, ` + bt + `admin` + bt + `@` + bt + `10.0.%` + bt +
		` PASSWORD HISTORY DEFAULT PASSWORD REUSE INTERVAL DEFAULT` +
		` PASSWORD REQUIRE CURRENT DEFAULT`
	stmt, err := ParseCreateUser(ddl, "8.0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stmt.DefaultRoles) != 3 {
		t.Fatalf("DefaultRoles length = %d, want 3: %+v", len(stmt.DefaultRoles), stmt.DefaultRoles)
	}
	want := []DefaultRole{
		{Name: "reader", Host: "%"},
		{Name: "writer", Host: "%"},
		{Name: "admin", Host: "10.0.%"},
	}
	for i, w := range want {
		if stmt.DefaultRoles[i] != w {
			t.Errorf("DefaultRoles[%d] = %+v, want %+v", i, stmt.DefaultRoles[i], w)
		}
	}
}

// TestParseCreateUser_DefaultRoleSingleQuoted covers the input-form
// variant where the role identifier uses single quotes instead of
// backticks. CREATE USER accepts both; the parser already accepts
// single-quoted user/host, so DEFAULT ROLE should too.
func TestParseCreateUser_DefaultRoleSingleQuoted(t *testing.T) {
	ddl := `CREATE USER 'u'@'%' IDENTIFIED WITH 'caching_sha2_password' AS '' REQUIRE NONE DEFAULT ROLE 'role1'@'%'`
	stmt, err := ParseCreateUser(ddl, "8.0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stmt.DefaultRoles) != 1 || stmt.DefaultRoles[0].Name != "role1" {
		t.Errorf("DefaultRoles = %+v, want [{role1 %%}]", stmt.DefaultRoles)
	}
}

// Ensure strings import is retained (used by BasicUser assertion).
var _ = strings.HasPrefix
