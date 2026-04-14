package opensearch

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// --- Unit tests (no cluster needed) ---

func TestInternalUserNormalize_sorts_lists(t *testing.T) {
	h := &internalUserHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_internal_user", Name: "test"},
		Body: buildMap(
			"backend_roles", provider.ListVal([]provider.Value{
				provider.StringVal("role_b"),
				provider.StringVal("role_a"),
			}),
			"opendistro_security_roles", provider.ListVal([]provider.Value{
				provider.StringVal("sec_z"),
				provider.StringVal("sec_a"),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	assertStringListOrder(t, result.Body, "backend_roles", []string{"role_a", "role_b"})
	assertStringListOrder(t, result.Body, "opendistro_security_roles", []string{"sec_a", "sec_z"})
}

func TestInternalUserNormalize_strips_empty_defaults(t *testing.T) {
	h := &internalUserHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_internal_user", Name: "test"},
		Body: buildMap(
			"backend_roles", provider.ListVal(nil),
			"opendistro_security_roles", provider.ListVal(nil),
			"description", provider.StringVal(""),
			"attributes", provider.MapVal(provider.NewOrderedMap()),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	if _, ok := result.Body.Get("backend_roles"); ok {
		t.Error("expected backend_roles to be stripped")
	}
	if _, ok := result.Body.Get("opendistro_security_roles"); ok {
		t.Error("expected opendistro_security_roles to be stripped")
	}
	if _, ok := result.Body.Get("description"); ok {
		t.Error("expected description to be stripped")
	}
	if _, ok := result.Body.Get("attributes"); ok {
		t.Error("expected attributes to be stripped")
	}
}

func TestInternalUserNormalize_idempotent(t *testing.T) {
	h := &internalUserHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_internal_user", Name: "test"},
		Body: buildMap(
			"backend_roles", provider.ListVal([]provider.Value{
				provider.StringVal("role_b"),
				provider.StringVal("role_a"),
			}),
			"description", provider.StringVal("a test user"),
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

func TestInternalUserNormalize_preserves_password_and_hash(t *testing.T) {
	h := &internalUserHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_internal_user", Name: "test"},
		Body: buildMap(
			"password", provider.StringVal("secret123"),
			"hash", provider.StringVal("$2y$12$somebcrypthash"),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	pw, ok := result.Body.Get("password")
	if !ok || pw.Str != "secret123" {
		t.Errorf("expected password to be preserved, got %v", pw)
	}
	hash, ok := result.Body.Get("hash")
	if !ok || hash.Str != "$2y$12$somebcrypthash" {
		t.Errorf("expected hash to be preserved, got %v", hash)
	}
}

func TestInternalUserValidate_valid_user(t *testing.T) {
	h := &internalUserHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_internal_user", Name: "good_user"},
		Body: buildMap(
			"password", provider.StringVal("secret"),
			"backend_roles", provider.ListVal([]provider.Value{
				provider.StringVal("role1"),
			}),
			"description", provider.StringVal("a test user"),
			"attributes", provider.MapVal(buildMap(
				"dept", provider.StringVal("engineering"),
			)),
			"opendistro_security_roles", provider.ListVal([]provider.Value{
				provider.StringVal("all_access"),
			}),
		),
	}

	if err := h.Validate(context.Background(), r); err != nil {
		t.Errorf("expected valid user to pass, got: %v", err)
	}
}

func TestInternalUserValidate_unknown_attribute(t *testing.T) {
	h := &internalUserHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_internal_user", Name: "bad_user"},
		Body: buildMap(
			"bogus", provider.StringVal("unexpected"),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for unknown attribute")
	}
}

func TestInternalUserValidate_backend_roles_wrong_type(t *testing.T) {
	h := &internalUserHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_internal_user", Name: "bad_user"},
		Body: buildMap("backend_roles", provider.StringVal("not_a_list")),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-list backend_roles")
	}
}

func TestInternalUserValidate_attributes_wrong_type(t *testing.T) {
	h := &internalUserHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_internal_user", Name: "bad_user"},
		Body: buildMap("attributes", provider.StringVal("not_a_map")),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-map attributes")
	}
}

func TestInternalUserValidate_attributes_non_string_value(t *testing.T) {
	h := &internalUserHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_internal_user", Name: "bad_user"},
		Body: buildMap(
			"attributes", provider.MapVal(buildMap(
				"count", provider.IntVal(42),
			)),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-string value in attributes map")
	}
}

func TestInternalUserValidate_password_wrong_type(t *testing.T) {
	h := &internalUserHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_internal_user", Name: "bad_user"},
		Body: buildMap("password", provider.IntVal(12345)),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-string password")
	}
}

// --- Integration tests ---

func TestInternalUserHandler_Integration(t *testing.T) {
	client := newTestClient(t)
	h := &internalUserHandler{}
	userName := "datastorectl_test_user"
	cleanupResource(t, client, "opensearch_internal_user", userName)

	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_internal_user", Name: userName},
			Body: buildMap(
				"hash", provider.StringVal("$2y$12$88IFVl6IfIwCFh5aQYfOmuXVL9j2hz/GusQb35o.4sdTDAEMTOD.K"),
				"backend_roles", provider.ListVal([]provider.Value{
					provider.StringVal("role1"),
				}),
				"description", provider.StringVal("test user"),
			),
		}

		if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate failed: %v", err)
		}
		requireResourceExists(t, client, "opensearch_internal_user", userName)
	})

	t.Run("discover_after_create", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == userName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("expected to find user %q in discovered resources", userName)
		}

		// Verify expected fields present.
		if _, ok := found.Body.Get("backend_roles"); !ok {
			t.Error("discovered user missing backend_roles")
		}
		if _, ok := found.Body.Get("description"); !ok {
			t.Error("discovered user missing description")
		}

		// Verify metadata and hash are stripped.
		for _, key := range []string{"hash", "reserved", "hidden", "static"} {
			if _, ok := found.Body.Get(key); ok {
				t.Errorf("discovered user should not have %q", key)
			}
		}
	})

	t.Run("normalize_roundtrip", func(t *testing.T) {
		// DCL resource with keys in alphabetical order to match jsonToValue output.
		dclResource := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_internal_user", Name: userName},
			Body: buildMap(
				"backend_roles", provider.ListVal([]provider.Value{
					provider.StringVal("role1"),
				}),
				"description", provider.StringVal("test user"),
			),
		}

		// Discover and find our user.
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}
		var discovered provider.Resource
		for _, r := range resources {
			if r.ID.Name == userName {
				discovered = r
				break
			}
		}

		normalizedDCL, err := h.Normalize(ctx, dclResource)
		if err != nil {
			t.Fatalf("Normalize DCL resource failed: %v", err)
		}
		normalizedAPI, err := h.Normalize(ctx, discovered)
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
			ID: provider.ResourceID{Type: "opensearch_internal_user", Name: userName},
			Body: buildMap(
				"hash", provider.StringVal("$2y$12$88IFVl6IfIwCFh5aQYfOmuXVL9j2hz/GusQb35o.4sdTDAEMTOD.K"),
				"backend_roles", provider.ListVal([]provider.Value{
					provider.StringVal("role1"),
					provider.StringVal("role2"),
				}),
				"description", provider.StringVal("test user"),
			),
		}

		if err := h.Apply(ctx, client, provider.OpUpdate, r); err != nil {
			t.Fatalf("Apply OpUpdate failed: %v", err)
		}

		// Re-discover and verify the update.
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover after update failed: %v", err)
		}
		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == userName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("user %q not found after update", userName)
		}

		br, ok := found.Body.Get("backend_roles")
		if !ok {
			t.Fatal("backend_roles missing after update")
		}
		if len(br.List) != 2 {
			t.Errorf("expected 2 backend_roles after update, got %d", len(br.List))
		}
	})

	t.Run("delete", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_internal_user", Name: userName},
			Body: provider.NewOrderedMap(),
		}

		if err := h.Apply(ctx, client, provider.OpDelete, r); err != nil {
			t.Fatalf("Apply OpDelete failed: %v", err)
		}
		requireResourceNotExists(t, client, "opensearch_internal_user", userName)
	})

	t.Run("discover_excludes_reserved", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		for _, r := range resources {
			if r.ID.Name == "admin" {
				t.Error("discovered built-in reserved user \"admin\" — should have been filtered")
			}
			if v, ok := r.Body.Get("reserved"); ok {
				t.Errorf("user %q has 'reserved' key in body: %s", r.ID.Name, v)
			}
		}
	})
}
