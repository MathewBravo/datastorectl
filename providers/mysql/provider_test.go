package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// TestProviderRegistered confirms the init() side-effect wired the
// "mysql" entry into the central registry.
func TestProviderRegistered(t *testing.T) {
	f, ok := provider.Lookup("mysql")
	if !ok {
		t.Fatal(`provider.Lookup("mysql") returned ok=false; expected registered`)
	}
	p := f()
	if p == nil {
		t.Fatal("factory returned nil provider")
	}
}

// TestConfigureReturnsNotImplemented asserts the scaffold's Configure
// returns a deterministic "not implemented" diagnostic so callers get
// a clear signal that the provider is a placeholder.
func TestConfigureReturnsNotImplemented(t *testing.T) {
	f, _ := provider.Lookup("mysql")
	p := f()
	diags := p.Configure(context.Background(), nil)
	if !diags.HasErrors() {
		t.Fatal("expected Configure to return an error diagnostic, got none")
	}
	if !strings.Contains(strings.ToLower(diags[0].Message), "not implemented") {
		t.Errorf("expected diagnostic to mention 'not implemented', got: %q", diags[0].Message)
	}
}

// TestHandlersRegisteredForAllResourceTypes probes the handlers map via
// Validate — a registered type returns a handler-level error (scaffold
// stub), an unregistered type returns the central "is not supported"
// message. The test passes if all four types are registered.
func TestHandlersRegisteredForAllResourceTypes(t *testing.T) {
	f, _ := provider.Lookup("mysql")
	p := f()
	types := []string{
		"mysql_user",
		"mysql_grant",
		"mysql_role",
		"mysql_database",
	}
	for _, typ := range types {
		r := provider.Resource{ID: provider.ResourceID{Type: typ, Name: "probe"}}
		diags := p.Validate(context.Background(), r)
		for _, d := range diags {
			if strings.Contains(d.Message, "is not supported by the mysql provider") {
				t.Errorf("type %q: expected a registered handler, got 'is not supported' diagnostic", typ)
			}
		}
	}
}
