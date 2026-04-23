package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// TestConfigure_ConnectsToLiveCluster verifies Configure returns no
// diagnostics when pointed at a reachable MySQL server with correct
// credentials and a matching declared version. Skipped when no cluster
// is available.
func TestConfigure_ConnectsToLiveCluster(t *testing.T) {
	skipIfNoCluster(t)

	f, _ := provider.Lookup("mysql")
	p := f()
	cfg := configMap(
		"endpoint", testEndpoint,
		"auth", "password",
		"username", testUsername,
		"password", testPassword,
		"tls", "skip-verify",
		"version", testServerVersion,
	)
	if diags := p.Configure(context.Background(), cfg); diags.HasErrors() {
		t.Fatalf("Configure failed against live cluster: %v", diags)
	}
}

// TestConfigure_ServerVersionMismatch asserts that declaring the wrong
// major.minor produces a clear migration-aware diagnostic rather than
// silently connecting to the wrong cluster.
func TestConfigure_ServerVersionMismatch(t *testing.T) {
	skipIfNoCluster(t)

	// Pick the other supported version — if the server is 8.4, declare 8.0
	// and vice versa.
	wrongVersion := "8.0"
	if testServerVersion == "8.0" {
		wrongVersion = "8.4"
	}

	f, _ := provider.Lookup("mysql")
	p := f()
	cfg := configMap(
		"endpoint", testEndpoint,
		"auth", "password",
		"username", testUsername,
		"password", testPassword,
		"tls", "skip-verify",
		"version", wrongVersion,
	)
	diags := p.Configure(context.Background(), cfg)
	if !diags.HasErrors() {
		t.Fatal("expected mismatch diagnostic; Configure succeeded")
	}
	msg := diags[0].Message
	if !strings.Contains(msg, "server reports") || !strings.Contains(msg, wrongVersion) {
		t.Errorf("expected mismatch diagnostic to mention both sides; got: %q", msg)
	}
}
