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
