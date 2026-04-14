package opensearch

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// --- Unit tests (no cluster needed) ---

func TestIngestPipelineNormalize_preserves_processor_order(t *testing.T) {
	h := &ingestPipelineHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: "test"},
		Body: buildMap(
			"description", provider.StringVal("test pipeline"),
			"processors", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"set", provider.MapVal(buildMap(
						"field", provider.StringVal("field_a"),
						"value", provider.StringVal("1"),
					)),
				)),
				provider.MapVal(buildMap(
					"set", provider.MapVal(buildMap(
						"field", provider.StringVal("field_b"),
						"value", provider.StringVal("2"),
					)),
				)),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	procs, _ := result.Body.Get("processors")
	if len(procs.List) != 2 {
		t.Fatalf("expected 2 processors, got %d", len(procs.List))
	}

	// Verify order preserved: field_a first, field_b second.
	first := procs.List[0].Map
	setVal, _ := first.Get("set")
	fieldVal, _ := setVal.Map.Get("field")
	if fieldVal.Str != "field_a" {
		t.Errorf("expected first processor field to be field_a, got %q", fieldVal.Str)
	}
}

func TestIngestPipelineNormalize_strips_empty_description(t *testing.T) {
	h := &ingestPipelineHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: "test"},
		Body: buildMap(
			"description", provider.StringVal(""),
			"processors", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"set", provider.MapVal(buildMap(
						"field", provider.StringVal("f"),
						"value", provider.StringVal("v"),
					)),
				)),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	if _, ok := result.Body.Get("description"); ok {
		t.Error("expected empty description to be stripped")
	}
}

func TestIngestPipelineNormalize_idempotent(t *testing.T) {
	h := &ingestPipelineHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: "test"},
		Body: buildMap(
			"description", provider.StringVal("test"),
			"processors", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"set", provider.MapVal(buildMap(
						"field", provider.StringVal("f"),
						"value", provider.StringVal("v"),
					)),
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

func TestIngestPipelineValidate_valid_pipeline(t *testing.T) {
	h := &ingestPipelineHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: "good"},
		Body: buildMap(
			"description", provider.StringVal("test"),
			"processors", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"set", provider.MapVal(buildMap(
						"field", provider.StringVal("f"),
						"value", provider.StringVal("v"),
					)),
				)),
			}),
		),
	}

	if err := h.Validate(context.Background(), r); err != nil {
		t.Errorf("expected valid pipeline to pass, got: %v", err)
	}
}

func TestIngestPipelineValidate_missing_processors(t *testing.T) {
	h := &ingestPipelineHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: "bad"},
		Body: buildMap("description", provider.StringVal("no processors")),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for missing processors")
	}
}

func TestIngestPipelineValidate_processors_wrong_type(t *testing.T) {
	h := &ingestPipelineHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: "bad"},
		Body: buildMap("processors", provider.StringVal("not_a_list")),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-list processors")
	}
}

// --- Integration tests ---

func TestIngestPipelineHandler_Integration(t *testing.T) {
	client := newTestClient(t)
	h := &ingestPipelineHandler{}
	pipelineName := "datastorectl_test_pipeline"
	cleanupResource(t, client, "opensearch_ingest_pipeline", pipelineName)

	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: pipelineName},
			Body: buildMap(
				"description", provider.StringVal("test pipeline"),
				"processors", provider.ListVal([]provider.Value{
					provider.MapVal(buildMap(
						"set", provider.MapVal(buildMap(
							"field", provider.StringVal("test_field"),
							"value", provider.StringVal("test_value"),
						)),
					)),
				}),
			),
		}

		if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate failed: %v", err)
		}
		requireResourceExists(t, client, "opensearch_ingest_pipeline", pipelineName)
	})

	t.Run("discover_after_create", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == pipelineName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("expected to find pipeline %q in discovered resources", pipelineName)
		}

		if _, ok := found.Body.Get("processors"); !ok {
			t.Error("discovered pipeline missing processors")
		}
		if _, ok := found.Body.Get("description"); !ok {
			t.Error("discovered pipeline missing description")
		}
	})

	t.Run("normalize_roundtrip", func(t *testing.T) {
		// Discover and find our pipeline.
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}
		var discovered provider.Resource
		for _, r := range resources {
			if r.ID.Name == pipelineName {
				discovered = r
				break
			}
		}

		normalizedAPI, err := h.Normalize(ctx, discovered)
		if err != nil {
			t.Fatalf("Normalize discovered resource failed: %v", err)
		}

		dclResource := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: pipelineName},
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
		// Change the processor value.
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: pipelineName},
			Body: buildMap(
				"description", provider.StringVal("test pipeline"),
				"processors", provider.ListVal([]provider.Value{
					provider.MapVal(buildMap(
						"set", provider.MapVal(buildMap(
							"field", provider.StringVal("test_field"),
							"value", provider.StringVal("updated_value"),
						)),
					)),
				}),
			),
		}

		if err := h.Apply(ctx, client, provider.OpUpdate, r); err != nil {
			t.Fatalf("Apply OpUpdate failed: %v", err)
		}

		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover after update failed: %v", err)
		}
		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == pipelineName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("pipeline %q not found after update", pipelineName)
		}

		// Verify the processor was updated.
		procs, _ := found.Body.Get("processors")
		proc := procs.List[0].Map
		setVal, _ := proc.Get("set")
		val, _ := setVal.Map.Get("value")
		if val.Str != "updated_value" {
			t.Errorf("expected processor value to be updated_value, got %q", val.Str)
		}
	})

	t.Run("delete", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: pipelineName},
			Body: provider.NewOrderedMap(),
		}

		if err := h.Apply(ctx, client, provider.OpDelete, r); err != nil {
			t.Fatalf("Apply OpDelete failed: %v", err)
		}
		requireResourceNotExists(t, client, "opensearch_ingest_pipeline", pipelineName)
	})
}
