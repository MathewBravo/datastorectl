package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// userRes builds a mysql_user resource with the given body label, user,
// and host values. The body label matches the DCL block label pre-normalize.
// The caller sets the ID.Name to the post-normalize canonical form
// ("user@host") to mirror what the engine passes to ValidateResources.
func userRes(label, user, host string, rng dcl.Range) provider.Resource {
	body := provider.NewOrderedMap()
	body.Set("user", provider.StringVal(user))
	body.Set("host", provider.StringVal(host))
	// Store the original DCL label in the body so the diagnostic can
	// surface it. The mysql_user Normalize rewrites ID.Name; the engine
	// invokes ValidateResources with the post-normalize form. Tests
	// simulate that by setting ID.Name = user@host.
	_ = label
	return provider.Resource{
		ID:          provider.ResourceID{Type: "mysql_user", Name: user + "@" + host},
		Body:        body,
		SourceRange: rng,
	}
}

func roleRes(name string, rng dcl.Range) provider.Resource {
	body := provider.NewOrderedMap()
	return provider.Resource{
		ID:          provider.ResourceID{Type: "mysql_role", Name: name},
		Body:        body,
		SourceRange: rng,
	}
}

func rng(line int) dcl.Range {
	return dcl.Range{
		Start: dcl.Pos{Filename: "test.dcl", Line: line, Column: 1},
		End:   dcl.Pos{Filename: "test.dcl", Line: line, Column: 10},
	}
}

func TestProvider_ValidateResources(t *testing.T) {
	p := &Provider{}

	t.Run("no_duplicates_is_clean", func(t *testing.T) {
		diags := p.ValidateResources(context.Background(), []provider.Resource{
			userRes("app", "app", "%", rng(10)),
			userRes("admin", "admin", "%", rng(20)),
			roleRes("reporter", rng(30)),
		})
		if diags.HasErrors() {
			t.Errorf("expected no errors, got: %s", diags.Error())
		}
	})

	t.Run("same_user_different_host_is_clean", func(t *testing.T) {
		// MySQL treats (app, %) and (app, localhost) as distinct rows.
		diags := p.ValidateResources(context.Background(), []provider.Resource{
			userRes("app_wildcard", "app", "%", rng(10)),
			userRes("app_local", "app", "localhost", rng(20)),
		})
		if diags.HasErrors() {
			t.Errorf("expected no errors for same-user different-host, got: %s", diags.Error())
		}
	})

	t.Run("duplicate_user_host_tuple_errors", func(t *testing.T) {
		diags := p.ValidateResources(context.Background(), []provider.Resource{
			userRes("user1", "alice", "%", rng(10)),
			userRes("user2", "alice", "%", rng(20)),
		})
		if !diags.HasErrors() {
			t.Fatal("expected error for duplicate (user, host) tuple")
		}
		msg := diags.Error()
		// Message must name the shared identity.
		if !strings.Contains(msg, "alice") {
			t.Errorf("diagnostic should mention user name, got: %s", msg)
		}
		if !strings.Contains(msg, "%") {
			t.Errorf("diagnostic should mention host, got: %s", msg)
		}
		// Message must point at both source lines.
		if !strings.Contains(msg, "10") || !strings.Contains(msg, "20") {
			t.Errorf("diagnostic should reference both source lines (10 and 20), got: %s", msg)
		}
		// Message must include an actionable suggestion.
		joined := msg
		for _, d := range diags {
			joined += " " + d.Suggestion
		}
		if !strings.Contains(joined, "user") && !strings.Contains(joined, "host") {
			t.Errorf("diagnostic should suggest changing user or host, got: %s", joined)
		}
	})

	t.Run("user_role_collision_errors", func(t *testing.T) {
		diags := p.ValidateResources(context.Background(), []provider.Resource{
			userRes("app_user", "app", "%", rng(10)),
			roleRes("app", rng(20)),
		})
		if !diags.HasErrors() {
			t.Fatal("expected error for mysql_user/mysql_role name collision at host %")
		}
		msg := diags.Error()
		if !strings.Contains(msg, "mysql_user") || !strings.Contains(msg, "mysql_role") {
			t.Errorf("diagnostic should mention both types, got: %s", msg)
		}
		if !strings.Contains(msg, "app") {
			t.Errorf("diagnostic should include the colliding name, got: %s", msg)
		}
		// Must include an actionable suggestion.
		joined := msg
		for _, d := range diags {
			joined += " " + d.Suggestion
		}
		if !strings.Contains(joined, "rename") {
			t.Errorf("diagnostic should suggest renaming, got: %s", joined)
		}
	})

	t.Run("three_way_collision_names_all_sources", func(t *testing.T) {
		diags := p.ValidateResources(context.Background(), []provider.Resource{
			userRes("u1", "svc", "%", rng(10)),
			userRes("u2", "svc", "%", rng(20)),
			userRes("u3", "svc", "%", rng(30)),
		})
		if !diags.HasErrors() {
			t.Fatal("expected error for three-way duplicate")
		}
		msg := diags.Error()
		for _, line := range []string{"10", "20", "30"} {
			if !strings.Contains(msg, line) {
				t.Errorf("diagnostic should reference line %s, got: %s", line, msg)
			}
		}
	})

	t.Run("grants_and_databases_ignored", func(t *testing.T) {
		// mysql_grant and mysql_database participate in different
		// identity spaces. They must not trigger false positives.
		body := provider.NewOrderedMap()
		body.Set("user", provider.StringVal("app"))
		body.Set("host", provider.StringVal("%"))
		body.Set("on", provider.StringVal("db.*"))
		grant := provider.Resource{
			ID:   provider.ResourceID{Type: "mysql_grant", Name: "app@%:db.*"},
			Body: body,
		}
		dbBody := provider.NewOrderedMap()
		dbBody.Set("name", provider.StringVal("app"))
		db := provider.Resource{
			ID:   provider.ResourceID{Type: "mysql_database", Name: "app"},
			Body: dbBody,
		}
		diags := p.ValidateResources(context.Background(), []provider.Resource{
			userRes("u", "app", "%", rng(10)),
			grant,
			db,
		})
		if diags.HasErrors() {
			t.Errorf("grants/databases must not trigger identity collisions, got: %s", diags.Error())
		}
	})

	t.Run("missing_user_or_host_is_deferred", func(t *testing.T) {
		// Per-resource Validate catches missing fields. ValidateResources
		// should skip such resources so it does not emit a confusing
		// duplicate "@" identity.
		body := provider.NewOrderedMap()
		// user and host both missing.
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "mysql_user", Name: "broken"},
			Body: body,
		}
		diags := p.ValidateResources(context.Background(), []provider.Resource{
			r,
			userRes("u", "app", "%", rng(10)),
		})
		if diags.HasErrors() {
			t.Errorf("missing fields should be deferred to per-resource Validate, got: %s", diags.Error())
		}
	})
}
