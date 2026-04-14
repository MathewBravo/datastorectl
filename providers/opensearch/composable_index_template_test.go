package opensearch

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// --- Unit tests (no cluster needed) ---

func TestComposableIndexTemplateNormalize_sorts_lists(t *testing.T) {
	h := &composableIndexTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: "test"},
		Body: buildMap(
			"composed_of", provider.ListVal([]provider.Value{
				provider.StringVal("component_b"),
				provider.StringVal("component_a"),
			}),
			"index_patterns", provider.ListVal([]provider.Value{
				provider.StringVal("logs-*"),
				provider.StringVal("audit-*"),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	assertStringListOrder(t, result.Body, "index_patterns", []string{"audit-*", "logs-*"})
	assertStringListOrder(t, result.Body, "composed_of", []string{"component_a", "component_b"})
}

func TestComposableIndexTemplateNormalize_strips_version(t *testing.T) {
	h := &composableIndexTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: "test"},
		Body: buildMap(
			"index_patterns", provider.ListVal([]provider.Value{
				provider.StringVal("logs-*"),
			}),
			"version", provider.IntVal(1),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	if _, ok := result.Body.Get("version"); ok {
		t.Error("expected version to be stripped")
	}
}

func TestComposableIndexTemplateNormalize_strips_empty_composed_of(t *testing.T) {
	h := &composableIndexTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: "test"},
		Body: buildMap(
			"composed_of", provider.ListVal(nil),
			"index_patterns", provider.ListVal([]provider.Value{
				provider.StringVal("logs-*"),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	if _, ok := result.Body.Get("composed_of"); ok {
		t.Error("expected empty composed_of to be stripped")
	}
}

func TestComposableIndexTemplateNormalize_idempotent(t *testing.T) {
	h := &composableIndexTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: "test"},
		Body: buildMap(
			"composed_of", provider.ListVal([]provider.Value{
				provider.StringVal("component_b"),
				provider.StringVal("component_a"),
			}),
			"index_patterns", provider.ListVal([]provider.Value{
				provider.StringVal("logs-*"),
			}),
			"priority", provider.IntVal(100),
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

func TestComposableIndexTemplateValidate_valid_template(t *testing.T) {
	h := &composableIndexTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: "good_template"},
		Body: buildMap(
			"composed_of", provider.ListVal([]provider.Value{
				provider.StringVal("component1"),
			}),
			"index_patterns", provider.ListVal([]provider.Value{
				provider.StringVal("logs-*"),
			}),
			"priority", provider.IntVal(100),
			"template", provider.MapVal(buildMap(
				"settings", provider.MapVal(buildMap(
					"index", provider.MapVal(buildMap(
						"number_of_shards", provider.StringVal("1"),
					)),
				)),
			)),
		),
	}

	if err := h.Validate(context.Background(), r); err != nil {
		t.Errorf("expected valid template to pass, got: %v", err)
	}
}

func TestComposableIndexTemplateValidate_missing_index_patterns(t *testing.T) {
	h := &composableIndexTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: "bad_template"},
		Body: buildMap(
			"composed_of", provider.ListVal([]provider.Value{
				provider.StringVal("component1"),
			}),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for missing index_patterns")
	}
}

func TestComposableIndexTemplateValidate_empty_index_patterns(t *testing.T) {
	h := &composableIndexTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: "bad_template"},
		Body: buildMap(
			"index_patterns", provider.ListVal(nil),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for empty index_patterns")
	}
}

func TestComposableIndexTemplateValidate_index_patterns_wrong_type(t *testing.T) {
	h := &composableIndexTemplateHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_composable_index_template", Name: "bad_template"},
		Body: buildMap("index_patterns", provider.StringVal("not_a_list")),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-list index_patterns")
	}
}

func TestComposableIndexTemplateValidate_priority_wrong_type(t *testing.T) {
	h := &composableIndexTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: "bad_template"},
		Body: buildMap(
			"index_patterns", provider.ListVal([]provider.Value{
				provider.StringVal("logs-*"),
			}),
			"priority", provider.StringVal("high"),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-integer priority")
	}
}

// --- Integration tests ---

func TestComposableIndexTemplateHandler_Integration(t *testing.T) {
	client := newTestClient(t)
	ch := &componentTemplateHandler{}
	ih := &composableIndexTemplateHandler{}

	componentName := "datastorectl_test_comp_for_idx"
	templateName := "datastorectl_test_idx_template"

	// Clean up in reverse dependency order: index template first, then component.
	cleanupResource(t, client, "opensearch_composable_index_template", templateName)
	cleanupResource(t, client, "opensearch_component_template", componentName)

	ctx := context.Background()

	t.Run("create_prerequisite_component", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_component_template", Name: componentName},
			Body: buildMap(
				"template", provider.MapVal(buildMap(
					"settings", provider.MapVal(buildMap(
						"index", provider.MapVal(buildMap(
							"number_of_shards", provider.StringVal("1"),
						)),
					)),
				)),
			),
		}

		if err := ch.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate for prerequisite component template failed: %v", err)
		}
		requireResourceExists(t, client, "opensearch_component_template", componentName)
	})

	t.Run("create", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: templateName},
			Body: buildMap(
				"composed_of", provider.ListVal([]provider.Value{
					provider.StringVal(componentName),
				}),
				"index_patterns", provider.ListVal([]provider.Value{
					provider.StringVal("datastorectl-test-*"),
				}),
				"priority", provider.IntVal(50),
			),
		}

		if err := ih.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate failed: %v", err)
		}
		requireResourceExists(t, client, "opensearch_composable_index_template", templateName)
	})

	t.Run("discover_after_create", func(t *testing.T) {
		resources, err := ih.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == templateName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("expected to find template %q in discovered resources", templateName)
		}

		// Verify expected fields present.
		if _, ok := found.Body.Get("index_patterns"); !ok {
			t.Error("discovered template missing index_patterns")
		}
		if _, ok := found.Body.Get("composed_of"); !ok {
			t.Error("discovered template missing composed_of")
		}

		// Verify version is stripped.
		if _, ok := found.Body.Get("version"); ok {
			t.Error("discovered template should not have version")
		}
	})

	t.Run("normalize_roundtrip", func(t *testing.T) {
		// DCL resource with keys in alphabetical order.
		dclResource := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: templateName},
			Body: buildMap(
				"composed_of", provider.ListVal([]provider.Value{
					provider.StringVal(componentName),
				}),
				"index_patterns", provider.ListVal([]provider.Value{
					provider.StringVal("datastorectl-test-*"),
				}),
				"priority", provider.IntVal(50),
			),
		}

		// Discover and find our template.
		resources, err := ih.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}
		var discovered provider.Resource
		for _, r := range resources {
			if r.ID.Name == templateName {
				discovered = r
				break
			}
		}

		normalizedDCL, err := ih.Normalize(ctx, dclResource)
		if err != nil {
			t.Fatalf("Normalize DCL resource failed: %v", err)
		}
		normalizedAPI, err := ih.Normalize(ctx, discovered)
		if err != nil {
			t.Fatalf("Normalize discovered resource failed: %v", err)
		}

		if !normalizedDCL.Body.Equal(normalizedAPI.Body) {
			t.Errorf("normalized bodies do not match:\nDCL: %s\nAPI: %s",
				provider.MapVal(normalizedDCL.Body), provider.MapVal(normalizedAPI.Body))
		}
	})

	t.Run("update", func(t *testing.T) {
		// Add a second index pattern.
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_composable_index_template", Name: templateName},
			Body: buildMap(
				"composed_of", provider.ListVal([]provider.Value{
					provider.StringVal(componentName),
				}),
				"index_patterns", provider.ListVal([]provider.Value{
					provider.StringVal("datastorectl-test-*"),
					provider.StringVal("datastorectl-extra-*"),
				}),
				"priority", provider.IntVal(50),
			),
		}

		if err := ih.Apply(ctx, client, provider.OpUpdate, r); err != nil {
			t.Fatalf("Apply OpUpdate failed: %v", err)
		}

		// Re-discover and verify the update.
		resources, err := ih.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover after update failed: %v", err)
		}
		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == templateName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("template %q not found after update", templateName)
		}

		patterns, ok := found.Body.Get("index_patterns")
		if !ok {
			t.Fatal("index_patterns missing after update")
		}
		if len(patterns.List) != 2 {
			t.Errorf("expected 2 index_patterns after update, got %d", len(patterns.List))
		}
	})

	t.Run("delete", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_composable_index_template", Name: templateName},
			Body: provider.NewOrderedMap(),
		}

		if err := ih.Apply(ctx, client, provider.OpDelete, r); err != nil {
			t.Fatalf("Apply OpDelete failed: %v", err)
		}
		requireResourceNotExists(t, client, "opensearch_composable_index_template", templateName)
	})

	t.Run("discover_excludes_system", func(t *testing.T) {
		resources, err := ih.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		for _, r := range resources {
			if len(r.ID.Name) > 0 && r.ID.Name[0] == '.' {
				t.Errorf("discovered system template %q — dot-prefixed templates should be filtered", r.ID.Name)
			}
		}
	})
}
