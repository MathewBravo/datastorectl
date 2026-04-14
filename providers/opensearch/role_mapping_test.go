package opensearch

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// --- Unit tests (no cluster needed) ---

func TestRoleMappingNormalize_sorts_lists(t *testing.T) {
	h := &roleMappingHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role_mapping", Name: "test"},
		Body: buildMap(
			"backend_roles", provider.ListVal([]provider.Value{
				provider.StringVal("arn:aws:iam::222222222:role/beta"),
				provider.StringVal("arn:aws:iam::111111111:role/alpha"),
			}),
			"hosts", provider.ListVal([]provider.Value{
				provider.StringVal("host-z"),
				provider.StringVal("host-a"),
			}),
			"users", provider.ListVal([]provider.Value{
				provider.StringVal("user_b"),
				provider.StringVal("user_a"),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	assertStringListOrder(t, result.Body, "backend_roles", []string{
		"arn:aws:iam::111111111:role/alpha",
		"arn:aws:iam::222222222:role/beta",
	})
	assertStringListOrder(t, result.Body, "hosts", []string{"host-a", "host-z"})
	assertStringListOrder(t, result.Body, "users", []string{"user_a", "user_b"})
}

func TestRoleMappingNormalize_strips_empty_defaults(t *testing.T) {
	h := &roleMappingHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role_mapping", Name: "test"},
		Body: buildMap(
			"backend_roles", provider.ListVal(nil),
			"hosts", provider.ListVal(nil),
			"users", provider.ListVal(nil),
			"and_backend_roles", provider.ListVal(nil),
			"description", provider.StringVal(""),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	for _, key := range []string{"backend_roles", "hosts", "users", "and_backend_roles", "description"} {
		if _, ok := result.Body.Get(key); ok {
			t.Errorf("expected %s to be stripped", key)
		}
	}
}

func TestRoleMappingNormalize_idempotent(t *testing.T) {
	h := &roleMappingHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role_mapping", Name: "test"},
		Body: buildMap(
			"backend_roles", provider.ListVal([]provider.Value{
				provider.StringVal("role_b"),
				provider.StringVal("role_a"),
			}),
			"description", provider.StringVal("a test mapping"),
		),
	}

	first, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("first Normalize failed: %v", err)
	}
	second, err := h.Normalize(context.Background(), first)
	if err != nil {
		t.Fatalf("second Normalize failed: %v", err)
	}

	if !first.Body.Equal(second.Body) {
		t.Errorf("Normalize is not idempotent:\nfirst:  %s\nsecond: %s",
			provider.MapVal(first.Body), provider.MapVal(second.Body))
	}
}

func TestRoleMappingValidate_valid_mapping(t *testing.T) {
	h := &roleMappingHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role_mapping", Name: "good_mapping"},
		Body: buildMap(
			"backend_roles", provider.ListVal([]provider.Value{
				provider.StringVal("arn:aws:iam::123456789:role/test"),
			}),
			"hosts", provider.ListVal([]provider.Value{
				provider.StringVal("*.example.com"),
			}),
			"users", provider.ListVal([]provider.Value{
				provider.StringVal("admin"),
			}),
			"and_backend_roles", provider.ListVal([]provider.Value{
				provider.StringVal("extra_role"),
			}),
			"description", provider.StringVal("a test mapping"),
		),
	}

	if err := h.Validate(context.Background(), r); err != nil {
		t.Errorf("expected valid mapping to pass, got: %v", err)
	}
}

func TestRoleMappingValidate_unknown_attribute(t *testing.T) {
	h := &roleMappingHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role_mapping", Name: "bad_mapping"},
		Body: buildMap(
			"bogus", provider.StringVal("unexpected"),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for unknown attribute")
	}
}

func TestRoleMappingValidate_backend_roles_wrong_type(t *testing.T) {
	h := &roleMappingHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_role_mapping", Name: "bad_mapping"},
		Body: buildMap("backend_roles", provider.StringVal("not_a_list")),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-list backend_roles")
	}
}

func TestRoleMappingValidate_description_wrong_type(t *testing.T) {
	h := &roleMappingHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_role_mapping", Name: "bad_mapping"},
		Body: buildMap("description", provider.IntVal(42)),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-string description")
	}
}

// --- Integration tests ---

func TestRoleMappingHandler_Integration(t *testing.T) {
	client := newTestClient(t)
	rh := &roleHandler{}
	mh := &roleMappingHandler{}
	roleName := "datastorectl_test_mapping_role"
	mappingName := roleName // mapping named after the role
	cleanupResource(t, client, "opensearch_role_mapping", mappingName)
	cleanupResource(t, client, "opensearch_role", roleName)

	ctx := context.Background()

	t.Run("create_prerequisite_role", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_role", Name: roleName},
			Body: buildMap(
				"cluster_permissions", provider.ListVal([]provider.Value{
					provider.StringVal("cluster_monitor"),
				}),
			),
		}

		if err := rh.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate for prerequisite role failed: %v", err)
		}
		requireResourceExists(t, client, "opensearch_role", roleName)
	})

	t.Run("create", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_role_mapping", Name: mappingName},
			Body: buildMap(
				"backend_roles", provider.ListVal([]provider.Value{
					provider.StringVal("arn:aws:iam::123456789:role/test"),
				}),
				"description", provider.StringVal("test mapping"),
			),
		}

		if err := mh.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate failed: %v", err)
		}
		requireResourceExists(t, client, "opensearch_role_mapping", mappingName)
	})

	t.Run("discover_after_create", func(t *testing.T) {
		resources, err := mh.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == mappingName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("expected to find mapping %q in discovered resources", mappingName)
		}

		// Verify expected fields present.
		if _, ok := found.Body.Get("backend_roles"); !ok {
			t.Error("discovered mapping missing backend_roles")
		}
		if _, ok := found.Body.Get("description"); !ok {
			t.Error("discovered mapping missing description")
		}

		// Verify metadata is stripped.
		for _, key := range []string{"reserved", "hidden", "static"} {
			if _, ok := found.Body.Get(key); ok {
				t.Errorf("discovered mapping should not have %q", key)
			}
		}
	})

	t.Run("normalize_roundtrip", func(t *testing.T) {
		// DCL resource with keys in alphabetical order to match jsonToValue output.
		dclResource := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_role_mapping", Name: mappingName},
			Body: buildMap(
				"backend_roles", provider.ListVal([]provider.Value{
					provider.StringVal("arn:aws:iam::123456789:role/test"),
				}),
				"description", provider.StringVal("test mapping"),
			),
		}

		// Discover and find our mapping.
		resources, err := mh.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}
		var discovered provider.Resource
		for _, r := range resources {
			if r.ID.Name == mappingName {
				discovered = r
				break
			}
		}

		normalizedDCL, err := mh.Normalize(ctx, dclResource)
		if err != nil {
			t.Fatalf("Normalize DCL resource failed: %v", err)
		}
		normalizedAPI, err := mh.Normalize(ctx, discovered)
		if err != nil {
			t.Fatalf("Normalize discovered resource failed: %v", err)
		}

		if !normalizedDCL.Body.Equal(normalizedAPI.Body) {
			t.Errorf("normalized bodies do not match:\nDCL: %s\nAPI: %s",
				provider.MapVal(normalizedDCL.Body), provider.MapVal(normalizedAPI.Body))
		}
	})

	t.Run("update", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_role_mapping", Name: mappingName},
			Body: buildMap(
				"backend_roles", provider.ListVal([]provider.Value{
					provider.StringVal("arn:aws:iam::123456789:role/test"),
				}),
				"users", provider.ListVal([]provider.Value{
					provider.StringVal("test_user"),
				}),
				"description", provider.StringVal("test mapping"),
			),
		}

		if err := mh.Apply(ctx, client, provider.OpUpdate, r); err != nil {
			t.Fatalf("Apply OpUpdate failed: %v", err)
		}

		// Re-discover and verify the update.
		resources, err := mh.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover after update failed: %v", err)
		}
		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == mappingName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("mapping %q not found after update", mappingName)
		}

		users, ok := found.Body.Get("users")
		if !ok {
			t.Fatal("users missing after update")
		}
		if len(users.List) != 1 {
			t.Errorf("expected 1 user after update, got %d", len(users.List))
		}
	})

	t.Run("delete", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_role_mapping", Name: mappingName},
			Body: provider.NewOrderedMap(),
		}

		if err := mh.Apply(ctx, client, provider.OpDelete, r); err != nil {
			t.Fatalf("Apply OpDelete failed: %v", err)
		}
		requireResourceNotExists(t, client, "opensearch_role_mapping", mappingName)
	})

	t.Run("discover_excludes_reserved", func(t *testing.T) {
		resources, err := mh.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		for _, r := range resources {
			if v, ok := r.Body.Get("reserved"); ok {
				t.Errorf("mapping %q has 'reserved' key in body: %s", r.ID.Name, v)
			}
			if v, ok := r.Body.Get("hidden"); ok {
				t.Errorf("mapping %q has 'hidden' key in body: %s", r.ID.Name, v)
			}
			if v, ok := r.Body.Get("static"); ok {
				t.Errorf("mapping %q has 'static' key in body: %s", r.ID.Name, v)
			}
		}
	})
}
