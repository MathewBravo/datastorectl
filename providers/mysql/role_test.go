package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// TestRoleHandler_Validate covers per-resource rules that don't need
// a cluster: non-empty name, no backticks in name, reasonable host.
func TestRoleHandler_Validate(t *testing.T) {
	h := &roleHandler{}
	cases := []struct {
		name     string
		resource provider.Resource
		wantErr  string
	}{
		{
			name: "empty name",
			resource: provider.Resource{
				ID: provider.ResourceID{Type: "mysql_role", Name: ""},
			},
			wantErr: "name cannot be empty",
		},
		{
			name: "backtick in name",
			resource: provider.Resource{
				ID: provider.ResourceID{Type: "mysql_role", Name: "bad`role"},
			},
			wantErr: "backtick",
		},
		{
			name: "valid name",
			resource: provider.Resource{
				ID: provider.ResourceID{Type: "mysql_role", Name: "reader"},
			},
		},
		{
			name:     "valid name with host override",
			resource: resourceWithBody("reader", "host", "%"),
		},
		{
			name:     "valid granted_roles list",
			resource: resourceWithGrantedRoles("advanced_reader", []string{"reader", "monitor"}),
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

// TestRoleHandler_CreateDiscoverDelete runs the full role lifecycle
// against a live cluster. Creates a role, discovers it, deletes.
func TestRoleHandler_CreateDiscoverDelete(t *testing.T) {
	client := newTestClient(t)
	h := &roleHandler{}
	ctx := context.Background()

	name := "dsctl_test_reader"
	t.Cleanup(func() {
		_, _ = client.DB().ExecContext(ctx, "DROP ROLE IF EXISTS `"+name+"`@`%`")
	})

	r := provider.Resource{
		ID:   provider.ResourceID{Type: "mysql_role", Name: name},
		Body: provider.NewOrderedMap(),
	}
	if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
		t.Fatalf("create: %v", err)
	}

	resources, err := h.Discover(ctx, client)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if findByName(resources, name) == nil {
		t.Fatalf("discover did not return role %q", name)
	}

	if err := h.Apply(ctx, client, provider.OpDelete, r); err != nil {
		t.Fatalf("delete: %v", err)
	}
	resources, _ = h.Discover(ctx, client)
	if findByName(resources, name) != nil {
		t.Errorf("role %q still present after delete", name)
	}
}

// TestRoleHandler_RoleToRoleGrants exercises the granted_roles path.
// Creates two roles, grants one to the other, rediscovers, revokes.
func TestRoleHandler_RoleToRoleGrants(t *testing.T) {
	client := newTestClient(t)
	h := &roleHandler{}
	ctx := context.Background()

	parent := "dsctl_test_parent_role"
	child := "dsctl_test_child_role"
	t.Cleanup(func() {
		_, _ = client.DB().ExecContext(ctx, "DROP ROLE IF EXISTS `"+child+"`@`%`, `"+parent+"`@`%`")
	})

	// Create both roles, grant parent to child via child's granted_roles.
	parentR := provider.Resource{
		ID:   provider.ResourceID{Type: "mysql_role", Name: parent},
		Body: provider.NewOrderedMap(),
	}
	if err := h.Apply(ctx, client, provider.OpCreate, parentR); err != nil {
		t.Fatalf("create parent: %v", err)
	}

	childR := resourceWithGrantedRoles(child, []string{parent})
	if err := h.Apply(ctx, client, provider.OpCreate, childR); err != nil {
		t.Fatalf("create child with granted_role: %v", err)
	}

	resources, err := h.Discover(ctx, client)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	found := findByName(resources, child)
	if found == nil {
		t.Fatalf("child role not found in discovery")
	}
	roles := getStringListField(found.Body, "granted_roles")
	if len(roles) != 1 || roles[0] != parent {
		t.Errorf("granted_roles = %v, want [%q]", roles, parent)
	}

	// Update child to have no granted roles (revoke).
	childR2 := resourceWithGrantedRoles(child, nil)
	if err := h.Apply(ctx, client, provider.OpUpdate, childR2); err != nil {
		t.Fatalf("update child to remove grants: %v", err)
	}

	resources, _ = h.Discover(ctx, client)
	found = findByName(resources, child)
	if found == nil {
		t.Fatalf("child role disappeared after update")
	}
	roles = getStringListField(found.Body, "granted_roles")
	if len(roles) != 0 {
		t.Errorf("granted_roles after revoke = %v, want empty", roles)
	}
}

// TestRoleHandler_DiscoverIdentifiesRolesNotUsers confirms a regular
// user account (non-locked, non-empty auth_string) is not returned as
// a role.
func TestRoleHandler_DiscoverIdentifiesRolesNotUsers(t *testing.T) {
	client := newTestClient(t)
	h := &roleHandler{}
	ctx := context.Background()

	userName := "dsctl_test_real_user"
	createTestUser(t, client, userName, "%", "pw")

	resources, err := h.Discover(ctx, client)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	for _, r := range resources {
		if r.ID.Name == userName {
			t.Errorf("regular user %q incorrectly classified as role", userName)
		}
	}
}

// resourceWithBody builds a minimal mysql_role resource with a single
// string-valued body field.
func resourceWithBody(name, key, value string) provider.Resource {
	body := provider.NewOrderedMap()
	body.Set(key, provider.StringVal(value))
	return provider.Resource{
		ID:   provider.ResourceID{Type: "mysql_role", Name: name},
		Body: body,
	}
}

// resourceWithGrantedRoles builds a mysql_role resource with a
// granted_roles list attribute.
func resourceWithGrantedRoles(name string, grantedRoles []string) provider.Resource {
	body := provider.NewOrderedMap()
	elems := make([]provider.Value, 0, len(grantedRoles))
	for _, r := range grantedRoles {
		elems = append(elems, provider.StringVal(r))
	}
	body.Set("granted_roles", provider.ListVal(elems))
	return provider.Resource{
		ID:   provider.ResourceID{Type: "mysql_role", Name: name},
		Body: body,
	}
}

// getStringListField extracts a list of strings from a resource body.
func getStringListField(body *provider.OrderedMap, key string) []string {
	v, ok := body.Get(key)
	if !ok || v.Kind != provider.KindList {
		return nil
	}
	out := make([]string, 0, len(v.List))
	for _, e := range v.List {
		if e.Kind == provider.KindString {
			out = append(out, e.Str)
		}
	}
	return out
}
