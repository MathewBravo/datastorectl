package mysql

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// TestConfigure_ConnectsToLiveCluster verifies Configure returns no
// diagnostics when pointed at a reachable MySQL server with correct
// credentials. Skipped automatically when no cluster is available.
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
	)
	if diags := p.Configure(context.Background(), cfg); diags.HasErrors() {
		t.Fatalf("Configure failed against live cluster: %v", diags)
	}
}
