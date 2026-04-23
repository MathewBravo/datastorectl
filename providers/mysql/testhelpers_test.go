package mysql

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// newTestClient opens a *Client against the live integration cluster
// using the shared test credentials. Skips t when no cluster is
// available. Registers a Cleanup that closes the pool.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	skipIfNoCluster(t)

	cfg := mysql.NewConfig()
	cfg.User = testUsername
	cfg.Passwd = testPassword
	cfg.Net = "tcp"
	cfg.Addr = testEndpoint
	cfg.TLSConfig = "skip-verify"
	cfg.AllowNativePasswords = true

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatalf("failed to open mysql test client: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	t.Cleanup(func() {
		_ = db.Close()
	})

	return &Client{db: db}
}

// createTestDatabase creates a fresh schema and registers a Cleanup
// that drops it. Name collisions are the caller's problem — pass a
// unique name per test.
func createTestDatabase(t *testing.T, c *Client, name string) {
	t.Helper()
	if _, err := c.db.Exec(fmt.Sprintf("CREATE DATABASE `%s`", name)); err != nil {
		t.Fatalf("createTestDatabase %q: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := c.db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", name)); err != nil {
			t.Logf("cleanup: DROP DATABASE %q: %v", name, err)
		}
	})
}

// createTestUser creates a fresh user with the given host pattern and
// password, then registers a Cleanup that drops it. Uses the
// caching_sha2_password plugin to match MySQL 8 defaults.
func createTestUser(t *testing.T, c *Client, user, host, password string) {
	t.Helper()
	stmt := fmt.Sprintf(
		"CREATE USER '%s'@'%s' IDENTIFIED WITH caching_sha2_password BY '%s'",
		escapeIdent(user), escapeIdent(host), escapeString(password),
	)
	if _, err := c.db.Exec(stmt); err != nil {
		t.Fatalf("createTestUser %s@%s: %v", user, host, err)
	}
	t.Cleanup(func() {
		drop := fmt.Sprintf("DROP USER IF EXISTS '%s'@'%s'", escapeIdent(user), escapeIdent(host))
		if _, err := c.db.Exec(drop); err != nil {
			t.Logf("cleanup: DROP USER %s@%s: %v", user, host, err)
		}
	})
}

// escapeIdent performs minimal SQL quoting for identifiers used inside
// single-quoted name fields like 'user'@'host'. Rejects names
// containing single quotes since tests shouldn't pass such inputs.
func escapeIdent(s string) string {
	for _, r := range s {
		if r == '\'' || r == '\\' {
			panic(fmt.Sprintf("test helper: unsafe character in identifier %q", s))
		}
	}
	return s
}

// escapeString is the same contract as escapeIdent but named to
// document the intent at the call site.
func escapeString(s string) string {
	return escapeIdent(s)
}
