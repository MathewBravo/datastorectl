package opensearch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// --- Unit tests (no cluster needed) ---

func TestNormalize_sorts_permissions(t *testing.T) {
	h := &roleHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role", Name: "test"},
		Body: buildMap(
			"cluster_permissions", provider.ListVal([]provider.Value{
				provider.StringVal("cluster_composite_ops"),
				provider.StringVal("cluster_monitor"),
				provider.StringVal("cluster_all"),
			}),
			"index_permissions", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"index_patterns", provider.ListVal([]provider.Value{
						provider.StringVal("z-index"),
						provider.StringVal("a-index"),
					}),
					"allowed_actions", provider.ListVal([]provider.Value{
						provider.StringVal("write"),
						provider.StringVal("read"),
					}),
					"fls", provider.ListVal([]provider.Value{
						provider.StringVal("field_b"),
						provider.StringVal("field_a"),
					}),
					"masked_fields", provider.ListVal([]provider.Value{
						provider.StringVal("mask_z"),
						provider.StringVal("mask_a"),
					}),
				)),
			}),
			"tenant_permissions", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"tenant_patterns", provider.ListVal([]provider.Value{
						provider.StringVal("z-tenant"),
						provider.StringVal("a-tenant"),
					}),
					"allowed_actions", provider.ListVal([]provider.Value{
						provider.StringVal("kibana_all_write"),
						provider.StringVal("kibana_all_read"),
					}),
				)),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	// cluster_permissions should be sorted
	assertStringListOrder(t, result.Body, "cluster_permissions",
		[]string{"cluster_all", "cluster_composite_ops", "cluster_monitor"})

	// index_permissions fields should be sorted
	ip, _ := result.Body.Get("index_permissions")
	entry := ip.List[0].Map
	assertStringListOrder(t, entry, "index_patterns", []string{"a-index", "z-index"})
	assertStringListOrder(t, entry, "allowed_actions", []string{"read", "write"})
	assertStringListOrder(t, entry, "fls", []string{"field_a", "field_b"})
	assertStringListOrder(t, entry, "masked_fields", []string{"mask_a", "mask_z"})

	// tenant_permissions fields should be sorted
	tp, _ := result.Body.Get("tenant_permissions")
	tEntry := tp.List[0].Map
	assertStringListOrder(t, tEntry, "tenant_patterns", []string{"a-tenant", "z-tenant"})
	assertStringListOrder(t, tEntry, "allowed_actions", []string{"kibana_all_read", "kibana_all_write"})
}

func TestNormalize_strips_empty_defaults(t *testing.T) {
	h := &roleHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role", Name: "test"},
		Body: buildMap(
			"cluster_permissions", provider.ListVal(nil),
			"index_permissions", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"index_patterns", provider.ListVal([]provider.Value{provider.StringVal("my-index")}),
					"allowed_actions", provider.ListVal([]provider.Value{provider.StringVal("read")}),
					"fls", provider.ListVal(nil),
					"masked_fields", provider.ListVal(nil),
					"dls", provider.StringVal(""),
				)),
			}),
			"tenant_permissions", provider.ListVal(nil),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	// Empty top-level lists should be stripped.
	if _, ok := result.Body.Get("cluster_permissions"); ok {
		t.Error("expected cluster_permissions to be stripped")
	}
	if _, ok := result.Body.Get("tenant_permissions"); ok {
		t.Error("expected tenant_permissions to be stripped")
	}

	// The index_permissions entry should have fls, masked_fields, dls stripped.
	ip, ok := result.Body.Get("index_permissions")
	if !ok {
		t.Fatal("expected index_permissions to remain (has one entry)")
	}
	entry := ip.List[0].Map
	if _, ok := entry.Get("fls"); ok {
		t.Error("expected fls to be stripped from index_permissions entry")
	}
	if _, ok := entry.Get("masked_fields"); ok {
		t.Error("expected masked_fields to be stripped from index_permissions entry")
	}
	if _, ok := entry.Get("dls"); ok {
		t.Error("expected dls to be stripped from index_permissions entry")
	}
}

func TestNormalize_idempotent(t *testing.T) {
	h := &roleHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role", Name: "test"},
		Body: buildMap(
			"cluster_permissions", provider.ListVal([]provider.Value{
				provider.StringVal("cluster_composite_ops"),
				provider.StringVal("cluster_monitor"),
			}),
			"index_permissions", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"index_patterns", provider.ListVal([]provider.Value{provider.StringVal("b"), provider.StringVal("a")}),
					"allowed_actions", provider.ListVal([]provider.Value{provider.StringVal("write")}),
				)),
			}),
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

func TestValidate_valid_role(t *testing.T) {
	h := &roleHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role", Name: "good_role"},
		Body: buildMap(
			"cluster_permissions", provider.ListVal([]provider.Value{
				provider.StringVal("cluster_monitor"),
			}),
			"index_permissions", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"index_patterns", provider.ListVal([]provider.Value{provider.StringVal("my-index")}),
					"allowed_actions", provider.ListVal([]provider.Value{provider.StringVal("read")}),
					"dls", provider.StringVal("{\"match\": {\"field\": \"value\"}}"),
					"fls", provider.ListVal([]provider.Value{provider.StringVal("field1")}),
					"masked_fields", provider.ListVal([]provider.Value{provider.StringVal("ssn")}),
				)),
			}),
			"tenant_permissions", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"tenant_patterns", provider.ListVal([]provider.Value{provider.StringVal("my_tenant")}),
					"allowed_actions", provider.ListVal([]provider.Value{provider.StringVal("kibana_all_write")}),
				)),
			}),
		),
	}

	if err := h.Validate(context.Background(), r); err != nil {
		t.Errorf("expected valid role to pass, got: %v", err)
	}
}

func TestValidate_cluster_permissions_wrong_type(t *testing.T) {
	h := &roleHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_role", Name: "bad_role"},
		Body: buildMap("cluster_permissions", provider.StringVal("not_a_list")),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-list cluster_permissions")
	}
}

func TestValidate_index_permissions_not_maps(t *testing.T) {
	h := &roleHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role", Name: "bad_role"},
		Body: buildMap(
			"index_permissions", provider.ListVal([]provider.Value{
				provider.StringVal("not_a_map"),
			}),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-map index_permissions entries")
	}
}

func TestValidate_unknown_attribute(t *testing.T) {
	h := &roleHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_role", Name: "bad_role"},
		Body: buildMap(
			"bogus", provider.StringVal("unexpected"),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for unknown attribute")
	}
}

func TestJsonToValue_roundtrip(t *testing.T) {
	original := map[string]any{
		"cluster_permissions": []any{"cluster_monitor", "cluster_all"},
		"index_permissions": []any{
			map[string]any{
				"index_patterns":  []any{"logs-*", "metrics-*"},
				"allowed_actions": []any{"read", "search"},
				"dls":             "",
			},
		},
		"count": float64(42),
		"flag":  true,
	}

	val := jsonToValue(original)
	roundtripped := valueToJSON(val)

	origJSON, _ := json.Marshal(original)
	rtJSON, _ := json.Marshal(roundtripped)

	// Re-unmarshal both to compare as maps (JSON key ordering can differ in Marshal).
	var origMap, rtMap map[string]any
	json.Unmarshal(origJSON, &origMap)
	json.Unmarshal(rtJSON, &rtMap)

	origNorm, _ := json.Marshal(origMap)
	rtNorm, _ := json.Marshal(rtMap)

	if string(origNorm) != string(rtNorm) {
		t.Errorf("roundtrip mismatch:\noriginal:     %s\nroundtripped: %s", origNorm, rtNorm)
	}
}

// --- Integration tests ---

func TestRoleHandler_Integration(t *testing.T) {
	client := newTestClient(t)
	h := &roleHandler{}
	roleName := "datastorectl_test_role"
	cleanupResource(t, client, "opensearch_role", roleName)

	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_role", Name: roleName},
			Body: buildMap(
				"cluster_permissions", provider.ListVal([]provider.Value{
					provider.StringVal("cluster_monitor"),
				}),
				"index_permissions", provider.ListVal([]provider.Value{
					provider.MapVal(buildMap(
						"index_patterns", provider.ListVal([]provider.Value{provider.StringVal("logs-*")}),
						"allowed_actions", provider.ListVal([]provider.Value{provider.StringVal("read")}),
					)),
				}),
			),
		}

		if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate failed: %v", err)
		}
		requireResourceExists(t, client, "opensearch_role", roleName)
	})

	t.Run("discover_after_create", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == roleName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("expected to find role %q in discovered resources", roleName)
		}

		// Verify body has the expected keys.
		if _, ok := found.Body.Get("cluster_permissions"); !ok {
			t.Error("discovered role missing cluster_permissions")
		}
		if _, ok := found.Body.Get("index_permissions"); !ok {
			t.Error("discovered role missing index_permissions")
		}
	})

	t.Run("normalize_roundtrip", func(t *testing.T) {
		// Build the DCL-side resource with keys in alphabetical order
		// to match jsonToValue's sorted-key output from Discover.
		dclResource := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_role", Name: roleName},
			Body: buildMap(
				"cluster_permissions", provider.ListVal([]provider.Value{
					provider.StringVal("cluster_monitor"),
				}),
				"index_permissions", provider.ListVal([]provider.Value{
					provider.MapVal(buildMap(
						"allowed_actions", provider.ListVal([]provider.Value{provider.StringVal("read")}),
						"index_patterns", provider.ListVal([]provider.Value{provider.StringVal("logs-*")}),
					)),
				}),
			),
		}

		// Discover and find our role.
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}
		var discovered provider.Resource
		for _, r := range resources {
			if r.ID.Name == roleName {
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
			ID: provider.ResourceID{Type: "opensearch_role", Name: roleName},
			Body: buildMap(
				"cluster_permissions", provider.ListVal([]provider.Value{
					provider.StringVal("cluster_monitor"),
					provider.StringVal("cluster_composite_ops"),
				}),
				"index_permissions", provider.ListVal([]provider.Value{
					provider.MapVal(buildMap(
						"index_patterns", provider.ListVal([]provider.Value{provider.StringVal("logs-*")}),
						"allowed_actions", provider.ListVal([]provider.Value{provider.StringVal("read")}),
					)),
				}),
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
			if resources[i].ID.Name == roleName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("role %q not found after update", roleName)
		}

		cp, ok := found.Body.Get("cluster_permissions")
		if !ok {
			t.Fatal("cluster_permissions missing after update")
		}
		if len(cp.List) != 2 {
			t.Errorf("expected 2 cluster_permissions after update, got %d", len(cp.List))
		}
	})

	t.Run("delete", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_role", Name: roleName},
			Body: provider.NewOrderedMap(),
		}

		if err := h.Apply(ctx, client, provider.OpDelete, r); err != nil {
			t.Fatalf("Apply OpDelete failed: %v", err)
		}
		requireResourceNotExists(t, client, "opensearch_role", roleName)
	})

	t.Run("discover_excludes_reserved", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		builtins := map[string]bool{
			"all_access":   true,
			"kibana_user":  true,
		}

		for _, r := range resources {
			if builtins[r.ID.Name] {
				t.Errorf("discovered built-in reserved role %q — should have been filtered", r.ID.Name)
			}
			if v, ok := r.Body.Get("reserved"); ok {
				t.Errorf("role %q has 'reserved' key in body: %s", r.ID.Name, v)
			}
		}
	})
}

// --- test helpers ---

// buildMap constructs an *OrderedMap from alternating key, value pairs.
func buildMap(kvs ...any) *provider.OrderedMap {
	m := provider.NewOrderedMap()
	for i := 0; i < len(kvs); i += 2 {
		m.Set(kvs[i].(string), kvs[i+1].(provider.Value))
	}
	return m
}

// assertStringListOrder asserts a key in an OrderedMap holds a list of strings in the expected order.
func assertStringListOrder(t *testing.T, m *provider.OrderedMap, key string, expected []string) {
	t.Helper()
	v, ok := m.Get(key)
	if !ok {
		t.Errorf("expected key %q to exist", key)
		return
	}
	if v.Kind != provider.KindList {
		t.Errorf("expected %q to be a list, got %s", key, v.Kind)
		return
	}
	if len(v.List) != len(expected) {
		t.Errorf("%q: expected %d elements, got %d", key, len(expected), len(v.List))
		return
	}
	for i, want := range expected {
		if v.List[i].Str != want {
			t.Errorf("%q[%d]: expected %q, got %q", key, i, want, v.List[i].Str)
		}
	}
}
