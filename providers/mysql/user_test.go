package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
	"github.com/MathewBravo/datastorectl/providers/mysql/auth"
)

// TestUserHandler_Validate covers rules checkable without a cluster.
func TestUserHandler_Validate(t *testing.T) {
	h := &userHandler{}
	cases := []struct {
		name     string
		resource provider.Resource
		wantErr  string
	}{
		{
			name:     "missing user attribute",
			resource: userResource("app", map[string]provider.Value{"host": provider.StringVal("%")}),
			wantErr:  "user",
		},
		{
			name:     "missing host attribute",
			resource: userResource("app", map[string]provider.Value{"user": provider.StringVal("app")}),
			wantErr:  "host",
		},
		{
			name: "password and password_hash both set",
			resource: userResource("app", map[string]provider.Value{
				"user":          provider.StringVal("app"),
				"host":          provider.StringVal("%"),
				"password":      provider.StringVal("pw"),
				"password_hash": provider.StringVal("*hash"),
			}),
			wantErr: "cannot set both",
		},
		{
			name: "caching_sha2 with no password or hash",
			resource: userResource("app", map[string]provider.Value{
				"user":        provider.StringVal("app"),
				"host":        provider.StringVal("%"),
				"auth_plugin": provider.StringVal(auth.PluginCachingSHA2),
			}),
			wantErr: "requires either password",
		},
		{
			name: "aws_iam with local password rejected",
			resource: userResource("app", map[string]provider.Value{
				"user":        provider.StringVal("app"),
				"host":        provider.StringVal("%"),
				"auth_plugin": provider.StringVal(auth.PluginAWSIAM),
				"password":    provider.StringVal("pw"),
			}),
			wantErr: "does not accept a local password",
		},
		{
			name: "unsupported plugin rejected",
			resource: userResource("app", map[string]provider.Value{
				"user":        provider.StringVal("app"),
				"host":        provider.StringVal("%"),
				"auth_plugin": provider.StringVal("authentication_ldap_simple"),
			}),
			wantErr: "not supported",
		},
		{
			name: "valid cleartext caching_sha2",
			resource: userResource("app", map[string]provider.Value{
				"user":     provider.StringVal("app"),
				"host":     provider.StringVal("%"),
				"password": provider.StringVal("pw"),
			}),
		},
		{
			name: "valid password_hash + native",
			resource: userResource("legacy", map[string]provider.Value{
				"user":          provider.StringVal("legacy"),
				"host":          provider.StringVal("%"),
				"auth_plugin":   provider.StringVal(auth.PluginNativePassword),
				"password_hash": provider.StringVal("*1234"),
			}),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := h.Validate(context.Background(), c.resource)
			if c.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), c.wantErr)
			}
		})
	}
}

// TestUserHandler_Normalize confirms Normalize rewrites ID.Name to
// the canonical "user@host" form so declared and discovered resources
// match under the engine's by-ID diff.
func TestUserHandler_Normalize(t *testing.T) {
	h := &userHandler{}
	r := userResource("app_handle", map[string]provider.Value{
		"user": provider.StringVal("app"),
		"host": provider.StringVal("10.0.%"),
	})
	out, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if out.ID.Name != "app@10.0.%" {
		t.Errorf("ID.Name = %q, want \"app@10.0.%%\"", out.ID.Name)
	}
}

// TestUserHandler_CreateDiscoverDelete runs the lifecycle with a
// cleartext password. Confirms the user appears in discovery with
// matching attributes.
func TestUserHandler_CreateDiscoverDelete(t *testing.T) {
	client := newTestClient(t)
	h := &userHandler{}
	ctx := context.Background()

	name := "dsctl_user_test"
	host := "%"
	t.Cleanup(func() {
		_, _ = client.DB().ExecContext(ctx, "DROP USER IF EXISTS `"+name+"`@`"+host+"`")
	})

	r := userResource("app_handle", map[string]provider.Value{
		"user":     provider.StringVal(name),
		"host":     provider.StringVal(host),
		"password": provider.StringVal("test_pw_123"),
	})
	r, _ = h.Normalize(ctx, r)

	if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
		t.Fatalf("create: %v", err)
	}

	resources, err := h.Discover(ctx, client)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	found := findByName(resources, name+"@"+host)
	if found == nil {
		t.Fatalf("discover did not return %q@%q", name, host)
	}
	if getBodyString(found.Body, "user") != name {
		t.Errorf("user = %q, want %q", getBodyString(found.Body, "user"), name)
	}
	if getBodyString(found.Body, "host") != host {
		t.Errorf("host = %q, want %q", getBodyString(found.Body, "host"), host)
	}
	if getBodyString(found.Body, "auth_plugin") != auth.PluginCachingSHA2 {
		t.Errorf("auth_plugin = %q", getBodyString(found.Body, "auth_plugin"))
	}

	if err := h.Apply(ctx, client, provider.OpDelete, r); err != nil {
		t.Fatalf("delete: %v", err)
	}
	resources, _ = h.Discover(ctx, client)
	if findByName(resources, name+"@"+host) != nil {
		t.Errorf("user %q@%q still present after delete", name, host)
	}
}

// TestUserHandler_UpdateAttributes confirms an ALTER USER reflects
// account_locked and resource-limit changes.
func TestUserHandler_UpdateAttributes(t *testing.T) {
	client := newTestClient(t)
	h := &userHandler{}
	ctx := context.Background()

	name := "dsctl_user_update"
	host := "%"
	t.Cleanup(func() {
		_, _ = client.DB().ExecContext(ctx, "DROP USER IF EXISTS `"+name+"`@`"+host+"`")
	})

	create := userResource("u", map[string]provider.Value{
		"user":     provider.StringVal(name),
		"host":     provider.StringVal(host),
		"password": provider.StringVal("pw"),
	})
	create, _ = h.Normalize(ctx, create)
	if err := h.Apply(ctx, client, provider.OpCreate, create); err != nil {
		t.Fatalf("create: %v", err)
	}

	update := userResource("u", map[string]provider.Value{
		"user":                  provider.StringVal(name),
		"host":                  provider.StringVal(host),
		"password":              provider.StringVal("pw"),
		"account_locked":        provider.BoolVal(true),
		"max_queries_per_hour":  provider.IntVal(1000),
		"max_user_connections":  provider.IntVal(5),
	})
	update, _ = h.Normalize(ctx, update)
	if err := h.Apply(ctx, client, provider.OpUpdate, update); err != nil {
		t.Fatalf("update: %v", err)
	}

	resources, _ := h.Discover(ctx, client)
	found := findByName(resources, name+"@"+host)
	if found == nil {
		t.Fatalf("user disappeared after update")
	}
	if !getBodyBool(found.Body, "account_locked") {
		t.Error("account_locked = false, want true")
	}
	if getBodyInt(found.Body, "max_queries_per_hour") != 1000 {
		t.Errorf("max_queries_per_hour = %d, want 1000", getBodyInt(found.Body, "max_queries_per_hour"))
	}
}

// TestUserHandler_DiscoverFiltersRoles confirms MySQL 8 roles (users
// with account_locked='Y' AND authentication_string='') are excluded.
func TestUserHandler_DiscoverFiltersRoles(t *testing.T) {
	client := newTestClient(t)
	h := &userHandler{}
	ctx := context.Background()

	roleName := "dsctl_test_role_filter"
	t.Cleanup(func() {
		_, _ = client.DB().ExecContext(ctx, "DROP ROLE IF EXISTS `"+roleName+"`@`%`")
	})

	if _, err := client.DB().ExecContext(ctx, "CREATE ROLE `"+roleName+"`@`%`"); err != nil {
		t.Fatalf("create role: %v", err)
	}

	resources, err := h.Discover(ctx, client)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if findByName(resources, roleName+"@%") != nil {
		t.Errorf("role %q leaked into user discovery", roleName)
	}
}

// userResource is a test helper that builds a Resource for mysql_user.
// The id_name is used as the initial ID.Name; Normalize will rewrite.
func userResource(handle string, body map[string]provider.Value) provider.Resource {
	om := provider.NewOrderedMap()
	for k, v := range body {
		om.Set(k, v)
	}
	return provider.Resource{
		ID:   provider.ResourceID{Type: "mysql_user", Name: handle},
		Body: om,
	}
}

