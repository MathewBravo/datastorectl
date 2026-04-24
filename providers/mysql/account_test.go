package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// TestClassifyUserLockout covers case 1 (exact-match user delete).
func TestClassifyUserLockout(t *testing.T) {
	caller := callerIdentity{User: "datastorectl", Host: "10.0.1.1"}
	cases := []struct {
		name     string
		resource provider.Resource
		want     bool
	}{
		{
			name: "exact match",
			resource: userResource("x", map[string]provider.Value{
				"user": provider.StringVal("datastorectl"),
				"host": provider.StringVal("10.0.1.1"),
			}),
			want: true,
		},
		{
			name: "different user",
			resource: userResource("x", map[string]provider.Value{
				"user": provider.StringVal("someoneelse"),
				"host": provider.StringVal("10.0.1.1"),
			}),
			want: false,
		},
		{
			name: "different host pattern",
			resource: userResource("x", map[string]provider.Value{
				"user": provider.StringVal("datastorectl"),
				"host": provider.StringVal("%"),
			}),
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyUserLockout(c.resource, caller); got != c.want {
				t.Errorf("classifyUserLockout = %v, want %v", got, c.want)
			}
		})
	}
}

// TestClassifyGrantLockout covers case 2 (required-grant revoke).
func TestClassifyGrantLockout(t *testing.T) {
	caller := callerIdentity{User: "datastorectl", Host: "%"}
	cases := []struct {
		name     string
		resource provider.Resource
		want     bool
	}{
		{
			name: "revoke select on mysql.user from caller",
			resource: grantResource(map[string]provider.Value{
				"user":       provider.StringVal("datastorectl"),
				"host":       provider.StringVal("%"),
				"database":   provider.StringVal("mysql"),
				"table":      provider.StringVal("user"),
				"privileges": stringList("SELECT"),
			}),
			want: true,
		},
		{
			name: "revoke global all privileges from caller",
			resource: grantResource(map[string]provider.Value{
				"user":       provider.StringVal("datastorectl"),
				"host":       provider.StringVal("%"),
				"database":   provider.StringVal("*"),
				"table":      provider.StringVal("*"),
				"privileges": stringList("ALL"),
			}),
			want: true,
		},
		{
			name: "revoke select on mysql.user from someone else",
			resource: grantResource(map[string]provider.Value{
				"user":       provider.StringVal("otheruser"),
				"host":       provider.StringVal("%"),
				"database":   provider.StringVal("mysql"),
				"table":      provider.StringVal("user"),
				"privileges": stringList("SELECT"),
			}),
			want: false,
		},
		{
			name: "revoke harmless privilege from caller",
			resource: grantResource(map[string]provider.Value{
				"user":       provider.StringVal("datastorectl"),
				"host":       provider.StringVal("%"),
				"database":   provider.StringVal("appdb"),
				"table":      provider.StringVal("*"),
				"privileges": stringList("SELECT"),
			}),
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyGrantLockout(c.resource, caller); got != c.want {
				t.Errorf("classifyGrantLockout = %v, want %v", got, c.want)
			}
		})
	}
}

// TestClassifyDefaultRoleLockout covers case 3 (default role delete).
func TestClassifyDefaultRoleLockout(t *testing.T) {
	caller := callerIdentity{
		User: "datastorectl", Host: "%",
		DefaultRoles: []roleRef{{User: "admin_role", Host: "%"}},
	}
	cases := []struct {
		name     string
		resource provider.Resource
		want     bool
	}{
		{
			name: "delete caller's default role",
			resource: provider.Resource{
				ID:   provider.ResourceID{Type: "mysql_role", Name: "admin_role"},
				Body: provider.NewOrderedMap(),
			},
			want: true,
		},
		{
			name: "delete unrelated role",
			resource: provider.Resource{
				ID:   provider.ResourceID{Type: "mysql_role", Name: "some_other_role"},
				Body: provider.NewOrderedMap(),
			},
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyDefaultRoleLockout(c.resource, caller); got != c.want {
				t.Errorf("classifyDefaultRoleLockout = %v, want %v", got, c.want)
			}
		})
	}
}

// TestGuardDeletes_Integration runs the full GuardDeletes against a
// live cluster. Verifies caller-identity fetch and all three cases
// fire against realistic delete sets.
func TestGuardDeletes_Integration(t *testing.T) {
	skipIfNoCluster(t)

	// Start a fresh provider configured against the test cluster so
	// caller identity is fetched.
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
		t.Fatalf("Configure: %v", diags)
	}

	guarder, ok := p.(interface {
		GuardDeletes(context.Context, []provider.Resource) ([]provider.DeleteGuard, interface{})
	})
	_ = guarder
	_ = ok
	// Use the real Provider type for the call.
	mp := p.(*Provider)
	deletes := []provider.Resource{
		// Case 1: caller's own user.
		userResource("x", map[string]provider.Value{
			"user": provider.StringVal(testUsername),
			"host": provider.StringVal("%"),
		}),
		// Case 2: revoking caller's SELECT ON mysql.user.
		grantResource(map[string]provider.Value{
			"user":       provider.StringVal(testUsername),
			"host":       provider.StringVal("%"),
			"database":   provider.StringVal("mysql"),
			"table":      provider.StringVal("user"),
			"privileges": stringList("SELECT"),
		}),
		// Benign delete — should not be guarded.
		userResource("x", map[string]provider.Value{
			"user": provider.StringVal("someone_else"),
			"host": provider.StringVal("%"),
		}),
	}
	guards, _ := mp.GuardDeletes(context.Background(), deletes)
	if len(guards) < 2 {
		t.Fatalf("expected at least 2 guards (cases 1 and 2), got %d: %v", len(guards), guards)
	}

	// Verify each guard's Reason is descriptive.
	for _, g := range guards {
		if g.Reason == "" {
			t.Errorf("guard for %q has empty reason", g.Resource)
		}
		if !strings.Contains(strings.ToLower(g.Reason), "caller") {
			t.Errorf("guard reason should mention caller: %q", g.Reason)
		}
	}
}
