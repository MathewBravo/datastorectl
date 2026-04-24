package parse

import (
	"strings"
	"testing"
)

// Real SHOW GRANTS output captured from a live mysql:8.4 container:
//
//   GRANT PROCESS, REPLICATION CLIENT ON *.* TO `app`@`10.0.%`
//   GRANT SELECT, INSERT, UPDATE, DELETE ON `appdb`.* TO `app`@`10.0.%`
//   GRANT ALL PRIVILEGES ON `appdb`.`users` TO `app`@`10.0.%` WITH GRANT OPTION

func TestParseGrant_GlobalScope(t *testing.T) {
	stmt, err := ParseGrant("GRANT PROCESS, REPLICATION CLIENT ON *.* TO `app`@`10.0.%`", "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.User != "app" || stmt.Host != "10.0.%" {
		t.Errorf("User=%q Host=%q", stmt.User, stmt.Host)
	}
	if stmt.Database != "*" || stmt.Table != "*" {
		t.Errorf("scope = (%q, %q), want (*, *)", stmt.Database, stmt.Table)
	}
	wantPrivs := []string{"PROCESS", "REPLICATION CLIENT"}
	if !equalStringSlices(stmt.Privileges, wantPrivs) {
		t.Errorf("privileges = %v, want %v", stmt.Privileges, wantPrivs)
	}
	if stmt.GrantOption {
		t.Error("GrantOption = true, want false")
	}
}

func TestParseGrant_SchemaScope(t *testing.T) {
	stmt, err := ParseGrant("GRANT SELECT, INSERT, UPDATE, DELETE ON `appdb`.* TO `app`@`10.0.%`", "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.Database != "appdb" || stmt.Table != "*" {
		t.Errorf("scope = (%q, %q), want (appdb, *)", stmt.Database, stmt.Table)
	}
	wantPrivs := []string{"DELETE", "INSERT", "SELECT", "UPDATE"} // sorted
	if !equalStringSlices(stmt.Privileges, wantPrivs) {
		t.Errorf("privileges = %v, want %v (should be sorted)", stmt.Privileges, wantPrivs)
	}
}

func TestParseGrant_TableScope(t *testing.T) {
	stmt, err := ParseGrant("GRANT ALL PRIVILEGES ON `appdb`.`users` TO `app`@`10.0.%` WITH GRANT OPTION", "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.Database != "appdb" || stmt.Table != "users" {
		t.Errorf("scope = (%q, %q), want (appdb, users)", stmt.Database, stmt.Table)
	}
	// ALL PRIVILEGES normalized to ALL
	if !equalStringSlices(stmt.Privileges, []string{"ALL"}) {
		t.Errorf("privileges = %v, want [ALL]", stmt.Privileges)
	}
	if !stmt.GrantOption {
		t.Error("GrantOption = false, want true")
	}
}

func TestParseGrant_AllAliasNormalizedToALL(t *testing.T) {
	// Both ALL and ALL PRIVILEGES collapse to ["ALL"] for deterministic diffs.
	for _, form := range []string{
		"GRANT ALL ON `db`.* TO `u`@`%`",
		"GRANT ALL PRIVILEGES ON `db`.* TO `u`@`%`",
	} {
		t.Run(form, func(t *testing.T) {
			stmt, err := ParseGrant(form, "8.4")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !equalStringSlices(stmt.Privileges, []string{"ALL"}) {
				t.Errorf("privileges = %v, want [ALL]", stmt.Privileges)
			}
		})
	}
}

func TestParseGrant_PrivilegesSortedAndUppercased(t *testing.T) {
	stmt, err := ParseGrant("GRANT update, select, insert ON `db`.* TO `u`@`%`", "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !equalStringSlices(stmt.Privileges, []string{"INSERT", "SELECT", "UPDATE"}) {
		t.Errorf("privileges = %v, want [INSERT SELECT UPDATE]", stmt.Privileges)
	}
}

func TestParseGrant_MultiWordPrivileges(t *testing.T) {
	ddl := "GRANT SELECT, SHOW DATABASES, CREATE TEMPORARY TABLES, REPLICATION CLIENT ON *.* TO `u`@`%`"
	stmt, err := ParseGrant(ddl, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"CREATE TEMPORARY TABLES", "REPLICATION CLIENT", "SELECT", "SHOW DATABASES"}
	if !equalStringSlices(stmt.Privileges, want) {
		t.Errorf("privileges = %v, want %v", stmt.Privileges, want)
	}
}

func TestParseGrant_ColumnGrantRejected(t *testing.T) {
	ddl := "GRANT SELECT (id, email) ON `db`.`users` TO `u`@`%`"
	_, err := ParseGrant(ddl, "8.4")
	if err == nil {
		t.Fatal("expected error for column-level grant")
	}
	if !strings.Contains(err.Error(), "column") {
		t.Errorf("error should mention column-level: %v", err)
	}
}

func TestParseGrant_RoutineGrantsRejected(t *testing.T) {
	cases := []string{
		"GRANT EXECUTE ON PROCEDURE `db`.`p` TO `u`@`%`",
		"GRANT EXECUTE ON FUNCTION `db`.`f` TO `u`@`%`",
	}
	for _, ddl := range cases {
		t.Run(ddl, func(t *testing.T) {
			_, err := ParseGrant(ddl, "8.4")
			if err == nil {
				t.Fatalf("expected error for routine grant: %q", ddl)
			}
			if !strings.Contains(err.Error(), "not supported") {
				t.Errorf("error should mention 'not supported': %v", err)
			}
		})
	}
}

func TestParseGrant_MalformedInputRejected(t *testing.T) {
	cases := []string{
		"",
		"NOT A GRANT STATEMENT",
		"GRANT",
		"GRANT SELECT",
		"GRANT SELECT ON",
		"GRANT SELECT ON db.tbl",
		"GRANT SELECT ON db.tbl TO",
		"GRANT SELECT ON db.tbl TO user", // missing @host
	}
	for i, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, err := ParseGrant(c, "8.4"); err == nil {
				t.Errorf("case %d (%q): expected error", i, c)
			}
		})
	}
}

func TestParseGrantLines_MultiStatement(t *testing.T) {
	output := `GRANT PROCESS, REPLICATION CLIENT ON *.* TO ` + bt + `app` + bt + `@` + bt + `10.0.%` + bt + `
GRANT SELECT, INSERT, UPDATE, DELETE ON ` + bt + `appdb` + bt + `.* TO ` + bt + `app` + bt + `@` + bt + `10.0.%` + bt + `
GRANT ALL PRIVILEGES ON ` + bt + `appdb` + bt + `.` + bt + `users` + bt + ` TO ` + bt + `app` + bt + `@` + bt + `10.0.%` + bt + ` WITH GRANT OPTION`
	stmts, err := ParseGrantLines(output, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stmts) != 3 {
		t.Fatalf("len(stmts) = %d, want 3", len(stmts))
	}
	if stmts[0].Database != "*" || stmts[0].Table != "*" {
		t.Errorf("stmts[0] scope = (%q, %q)", stmts[0].Database, stmts[0].Table)
	}
	if stmts[1].Database != "appdb" || stmts[1].Table != "*" {
		t.Errorf("stmts[1] scope = (%q, %q)", stmts[1].Database, stmts[1].Table)
	}
	if stmts[2].Database != "appdb" || stmts[2].Table != "users" {
		t.Errorf("stmts[2] scope = (%q, %q)", stmts[2].Database, stmts[2].Table)
	}
	if !stmts[2].GrantOption {
		t.Error("stmts[2] GrantOption = false, want true")
	}
}

func TestParseGrantLines_TolerantOfBlankLinesAndWhitespace(t *testing.T) {
	output := "\n\n   GRANT SELECT ON `db`.* TO `u`@`%`   \n\n   GRANT INSERT ON `db`.* TO `u`@`%`\n   "
	stmts, err := ParseGrantLines(output, "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("len(stmts) = %d, want 2", len(stmts))
	}
}

func TestParseGrant_SingleQuotedIdents(t *testing.T) {
	// User-authored DCL uses single quotes instead of backticks.
	stmt, err := ParseGrant("GRANT SELECT ON 'appdb'.* TO 'app'@'10.0.%'", "8.4")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.User != "app" || stmt.Host != "10.0.%" || stmt.Database != "appdb" {
		t.Errorf("unexpected: user=%q host=%q db=%q", stmt.User, stmt.Host, stmt.Database)
	}
}

// TestParseGrant_RoleGrantAurora exercises the exact shape AWS RDS
// Aurora's SHOW GRANTS emits for its master `admin` user. Role-to-user
// grants have no privilege names and no ON clause. Surfaced by #201;
// tracked in #215.
func TestParseGrant_RoleGrantAurora(t *testing.T) {
	stmt, err := ParseGrant("GRANT `rds_superuser_role`@`%` TO `admin`@`%`", "8.0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !stmt.IsRoleGrant() {
		t.Error("IsRoleGrant() = false, want true")
	}
	if len(stmt.Privileges) != 0 {
		t.Errorf("Privileges = %v, want empty for role grant", stmt.Privileges)
	}
	if len(stmt.GrantedRoles) != 1 {
		t.Fatalf("GrantedRoles length = %d, want 1", len(stmt.GrantedRoles))
	}
	got := stmt.GrantedRoles[0]
	if got.Name != "rds_superuser_role" || got.Host != "%" {
		t.Errorf("GrantedRoles[0] = %+v, want {rds_superuser_role %%}", got)
	}
	if stmt.User != "admin" || stmt.Host != "%" {
		t.Errorf("recipient = %q@%q, want admin@%%", stmt.User, stmt.Host)
	}
}

// TestParseGrant_RoleGrantMultiple covers comma-separated roles.
func TestParseGrant_RoleGrantMultiple(t *testing.T) {
	stmt, err := ParseGrant("GRANT `reader`@`%`, `writer`@`%` TO `alice`@`10.0.%`", "8.0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !stmt.IsRoleGrant() {
		t.Error("IsRoleGrant() = false, want true")
	}
	if len(stmt.GrantedRoles) != 2 {
		t.Fatalf("GrantedRoles length = %d, want 2: %+v", len(stmt.GrantedRoles), stmt.GrantedRoles)
	}
	if stmt.GrantedRoles[0].Name != "reader" || stmt.GrantedRoles[1].Name != "writer" {
		t.Errorf("GrantedRoles names = %+v", stmt.GrantedRoles)
	}
}

// TestParseGrant_RoleGrantWithAdminOption covers the WITH ADMIN OPTION
// trailer that MySQL 8 allows on role grants (symmetric with WITH GRANT
// OPTION on privilege grants).
func TestParseGrant_RoleGrantWithAdminOption(t *testing.T) {
	stmt, err := ParseGrant("GRANT `admin_role`@`%` TO `ops`@`%` WITH ADMIN OPTION", "8.0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !stmt.IsRoleGrant() {
		t.Error("IsRoleGrant() = false, want true")
	}
	if !stmt.AdminOption {
		t.Error("AdminOption = false, want true")
	}
}

// TestParseGrant_PrivilegeGrantUnchanged is a regression check: the
// existing privilege-grant paths must stay exactly as before — no
// GrantedRoles populated, IsRoleGrant() false.
func TestParseGrant_PrivilegeGrantUnchanged(t *testing.T) {
	stmt, err := ParseGrant("GRANT SELECT ON `appdb`.* TO `app`@`%`", "8.0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stmt.IsRoleGrant() {
		t.Error("IsRoleGrant() = true on a privilege grant")
	}
	if len(stmt.GrantedRoles) != 0 {
		t.Errorf("GrantedRoles = %+v, want empty on a privilege grant", stmt.GrantedRoles)
	}
}

// TestParseGrantLines_MixedPrivilegeAndRole confirms ParseGrantLines
// returns both forms interleaved without error, preserving line order.
func TestParseGrantLines_MixedPrivilegeAndRole(t *testing.T) {
	output := "GRANT USAGE ON *.* TO `admin`@`%`\n" +
		"GRANT `rds_superuser_role`@`%` TO `admin`@`%`"
	stmts, err := ParseGrantLines(output, "8.0")
	if err != nil {
		t.Fatalf("ParseGrantLines: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("got %d stmts, want 2", len(stmts))
	}
	if stmts[0].IsRoleGrant() {
		t.Error("first line should be a privilege grant")
	}
	if !stmts[1].IsRoleGrant() {
		t.Error("second line should be a role grant")
	}
}

// equalStringSlices compares two string slices in order.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
