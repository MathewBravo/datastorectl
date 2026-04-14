package opensearch

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// --- Unit tests (no cluster needed) ---

func TestComponentTemplateNormalize_strips_version(t *testing.T) {
	h := &componentTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_component_template", Name: "test"},
		Body: buildMap(
			"template", provider.MapVal(buildMap(
				"settings", provider.MapVal(buildMap(
					"index", provider.MapVal(buildMap(
						"number_of_shards", provider.StringVal("1"),
					)),
				)),
			)),
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
	if _, ok := result.Body.Get("template"); !ok {
		t.Error("expected template to be preserved")
	}
}

func TestComponentTemplateNormalize_idempotent(t *testing.T) {
	h := &componentTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_component_template", Name: "test"},
		Body: buildMap(
			"template", provider.MapVal(buildMap(
				"mappings", provider.MapVal(buildMap(
					"properties", provider.MapVal(buildMap(
						"timestamp", provider.MapVal(buildMap(
							"type", provider.StringVal("date"),
						)),
					)),
				)),
			)),
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

func TestComponentTemplateValidate_valid_template(t *testing.T) {
	h := &componentTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_component_template", Name: "good_template"},
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

	if err := h.Validate(context.Background(), r); err != nil {
		t.Errorf("expected valid template to pass, got: %v", err)
	}
}

func TestComponentTemplateValidate_missing_template(t *testing.T) {
	h := &componentTemplateHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_component_template", Name: "bad_template"},
		Body: buildMap(
			"version", provider.IntVal(1),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for missing template")
	}
}

func TestComponentTemplateValidate_template_wrong_type(t *testing.T) {
	h := &componentTemplateHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_component_template", Name: "bad_template"},
		Body: buildMap("template", provider.StringVal("not_a_map")),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-map template")
	}
}

// --- Integration tests ---

func TestComponentTemplateHandler_Integration(t *testing.T) {
	client := newTestClient(t)
	h := &componentTemplateHandler{}
	templateName := "datastorectl_test_component"
	cleanupResource(t, client, "opensearch_component_template", templateName)

	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_component_template", Name: templateName},
			Body: buildMap(
				"template", provider.MapVal(buildMap(
					"settings", provider.MapVal(buildMap(
						"index", provider.MapVal(buildMap(
							"number_of_shards", provider.StringVal("2"),
						)),
					)),
				)),
			),
		}

		if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate failed: %v", err)
		}
		requireResourceExists(t, client, "opensearch_component_template", templateName)
	})

	t.Run("discover_after_create", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
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

		// Verify template block present.
		if _, ok := found.Body.Get("template"); !ok {
			t.Error("discovered template missing template block")
		}

		// Verify version is stripped.
		if _, ok := found.Body.Get("version"); ok {
			t.Error("discovered template should not have version")
		}
	})

	t.Run("normalize_roundtrip", func(t *testing.T) {
		// Discover and find our template.
		resources, err := h.Discover(ctx, client)
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

		normalizedAPI, err := h.Normalize(ctx, discovered)
		if err != nil {
			t.Fatalf("Normalize discovered resource failed: %v", err)
		}

		// Build DCL resource matching normalized discovered body.
		dclResource := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_component_template", Name: templateName},
			Body: normalizedAPI.Body.Clone(),
		}

		normalizedDCL, err := h.Normalize(ctx, dclResource)
		if err != nil {
			t.Fatalf("Normalize DCL resource failed: %v", err)
		}

		if !normalizedDCL.Body.Equal(normalizedAPI.Body) {
			t.Errorf("normalized bodies do not match:\nDCL: %s\nAPI: %s",
				provider.MapVal(normalizedDCL.Body), provider.MapVal(normalizedAPI.Body))
		}
	})

	t.Run("update", func(t *testing.T) {
		// Add mappings to the template.
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_component_template", Name: templateName},
			Body: buildMap(
				"template", provider.MapVal(buildMap(
					"mappings", provider.MapVal(buildMap(
						"properties", provider.MapVal(buildMap(
							"timestamp", provider.MapVal(buildMap(
								"type", provider.StringVal("date"),
							)),
						)),
					)),
					"settings", provider.MapVal(buildMap(
						"index", provider.MapVal(buildMap(
							"number_of_shards", provider.StringVal("2"),
						)),
					)),
				)),
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
			if resources[i].ID.Name == templateName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("template %q not found after update", templateName)
		}

		tmpl, _ := found.Body.Get("template")
		if _, ok := tmpl.Map.Get("mappings"); !ok {
			t.Error("mappings missing after update")
		}
	})

	t.Run("delete", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_component_template", Name: templateName},
			Body: provider.NewOrderedMap(),
		}

		if err := h.Apply(ctx, client, provider.OpDelete, r); err != nil {
			t.Fatalf("Apply OpDelete failed: %v", err)
		}
		requireResourceNotExists(t, client, "opensearch_component_template", templateName)
	})

	t.Run("discover_excludes_system", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
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
