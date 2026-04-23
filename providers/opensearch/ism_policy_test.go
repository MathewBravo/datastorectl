package opensearch

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// --- Unit tests (no cluster needed) ---

func TestISMPolicyNormalize_sorts_ism_template(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "test"},
		Body: buildMap(
			"default_state", provider.StringVal("hot"),
			"ism_template", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"index_patterns", provider.ListVal([]provider.Value{
						provider.StringVal("logs-*"),
					}),
					"priority", provider.IntVal(200),
				)),
				provider.MapVal(buildMap(
					"index_patterns", provider.ListVal([]provider.Value{
						provider.StringVal("audit-*"),
					}),
					"priority", provider.IntVal(100),
				)),
			}),
			"states", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"name", provider.StringVal("hot"),
				)),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	tmpl, ok := result.Body.Get("ism_template")
	if !ok {
		t.Fatal("expected ism_template to be present")
	}
	if len(tmpl.List) != 2 {
		t.Fatalf("expected 2 ism_template entries, got %d", len(tmpl.List))
	}

	// After sorting by JSON: audit-* (priority 100) comes before logs-* (priority 200).
	first := tmpl.List[0].Map
	prio, _ := first.Get("priority")
	if prio.Int != 100 {
		t.Errorf("expected first ism_template priority to be 100, got %d", prio.Int)
	}
}

func TestISMPolicyNormalize_preserves_state_order(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "test"},
		Body: buildMap(
			"default_state", provider.StringVal("hot"),
			"states", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap("name", provider.StringVal("hot"))),
				provider.MapVal(buildMap("name", provider.StringVal("warm"))),
				provider.MapVal(buildMap("name", provider.StringVal("delete"))),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	states, _ := result.Body.Get("states")
	expected := []string{"hot", "warm", "delete"}
	for i, want := range expected {
		name, _ := states.List[i].Map.Get("name")
		if name.Str != want {
			t.Errorf("states[%d]: expected %q, got %q", i, want, name.Str)
		}
	}
}

func TestISMPolicyNormalize_strips_metadata(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "test"},
		Body: buildMap(
			"default_state", provider.StringVal("hot"),
			"error_notification", provider.NullVal(),
			"last_updated_time", provider.IntVal(1612345678901),
			"policy_id", provider.StringVal("test"),
			"schema_version", provider.IntVal(1),
			"states", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap("name", provider.StringVal("hot"))),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	for _, key := range []string{"policy_id", "last_updated_time", "schema_version", "error_notification"} {
		if _, ok := result.Body.Get(key); ok {
			t.Errorf("expected %s to be stripped", key)
		}
	}
}

func TestISMPolicyNormalize_strips_null_ism_template(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "test"},
		Body: buildMap(
			"default_state", provider.StringVal("hot"),
			"ism_template", provider.NullVal(),
			"states", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap("name", provider.StringVal("hot"))),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	if _, ok := result.Body.Get("ism_template"); ok {
		t.Error("expected null ism_template to be stripped")
	}
}

func TestISMPolicyNormalize_strips_default_retry(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "test"},
		Body: buildMap(
			"default_state", provider.StringVal("hot"),
			"states", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"name", provider.StringVal("delete"),
					"actions", provider.ListVal([]provider.Value{
						provider.MapVal(buildMap(
							"delete", provider.MapVal(provider.NewOrderedMap()),
							"retry", provider.MapVal(buildMap(
								"count", provider.IntVal(3),
								"backoff", provider.StringVal("exponential"),
								"delay", provider.StringVal("1m"),
							)),
						)),
					}),
				)),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	states, _ := result.Body.Get("states")
	action := states.List[0].Map
	actions, _ := action.Get("actions")
	first := actions.List[0].Map
	if _, ok := first.Get("retry"); ok {
		t.Error("expected default retry to be stripped")
	}
	if _, ok := first.Get("delete"); !ok {
		t.Error("expected delete action to remain")
	}
}

func TestISMPolicyNormalize_preserves_custom_retry(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "test"},
		Body: buildMap(
			"default_state", provider.StringVal("hot"),
			"states", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"name", provider.StringVal("delete"),
					"actions", provider.ListVal([]provider.Value{
						provider.MapVal(buildMap(
							"delete", provider.MapVal(provider.NewOrderedMap()),
							"retry", provider.MapVal(buildMap(
								"count", provider.IntVal(5),
								"backoff", provider.StringVal("exponential"),
								"delay", provider.StringVal("1m"),
							)),
						)),
					}),
				)),
			}),
		),
	}

	result, err := h.Normalize(context.Background(), r)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	states, _ := result.Body.Get("states")
	action := states.List[0].Map
	actions, _ := action.Get("actions")
	first := actions.List[0].Map
	retry, ok := first.Get("retry")
	if !ok {
		t.Fatal("expected custom retry to be preserved")
	}
	count, _ := retry.Map.Get("count")
	if count.Int != 5 {
		t.Errorf("expected retry.count to be 5, got %d", count.Int)
	}
}

func TestISMPolicyNormalize_idempotent(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "test"},
		Body: buildMap(
			"default_state", provider.StringVal("hot"),
			"description", provider.StringVal("test policy"),
			"ism_template", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"index_patterns", provider.ListVal([]provider.Value{
						provider.StringVal("logs-*"),
					}),
					"priority", provider.IntVal(100),
				)),
			}),
			"states", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"name", provider.StringVal("hot"),
					"transitions", provider.ListVal([]provider.Value{
						provider.MapVal(buildMap(
							"conditions", provider.MapVal(buildMap(
								"min_index_age", provider.StringVal("7d"),
							)),
							"state_name", provider.StringVal("delete"),
						)),
					}),
				)),
				provider.MapVal(buildMap(
					"actions", provider.ListVal([]provider.Value{
						provider.MapVal(buildMap(
							"delete", provider.MapVal(provider.NewOrderedMap()),
						)),
					}),
					"name", provider.StringVal("delete"),
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

func TestISMPolicyValidate_valid_policy(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "good_policy"},
		Body: buildMap(
			"default_state", provider.StringVal("hot"),
			"description", provider.StringVal("test policy"),
			"states", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"name", provider.StringVal("hot"),
					"transitions", provider.ListVal([]provider.Value{
						provider.MapVal(buildMap(
							"conditions", provider.MapVal(buildMap(
								"min_index_age", provider.StringVal("7d"),
							)),
							"state_name", provider.StringVal("delete"),
						)),
					}),
				)),
				provider.MapVal(buildMap(
					"actions", provider.ListVal([]provider.Value{
						provider.MapVal(buildMap(
							"delete", provider.MapVal(provider.NewOrderedMap()),
						)),
					}),
					"name", provider.StringVal("delete"),
				)),
			}),
			"ism_template", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"index_patterns", provider.ListVal([]provider.Value{
						provider.StringVal("logs-*"),
					}),
					"priority", provider.IntVal(100),
				)),
			}),
		),
	}

	if err := h.Validate(context.Background(), r); err != nil {
		t.Errorf("expected valid policy to pass, got: %v", err)
	}
}

func TestISMPolicyValidate_missing_states(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "bad_policy"},
		Body: buildMap(
			"default_state", provider.StringVal("hot"),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for missing states")
	}
}

func TestISMPolicyValidate_empty_states(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "bad_policy"},
		Body: buildMap(
			"states", provider.ListVal(nil),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for empty states")
	}
}

func TestISMPolicyValidate_state_missing_name(t *testing.T) {
	h := &ismPolicyHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "bad_policy"},
		Body: buildMap(
			"states", provider.ListVal([]provider.Value{
				provider.MapVal(buildMap(
					"actions", provider.ListVal(nil),
				)),
			}),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for state missing name")
	}
}

// --- Integration tests ---

func TestISMPolicyHandler_Integration(t *testing.T) {
	client := newTestClient(t)
	h := &ismPolicyHandler{}
	policyName := "datastorectl_test_ism_policy"
	cleanupResource(t, client, "opensearch_ism_policy", policyName)

	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: policyName},
			Body: buildMap(
				"default_state", provider.StringVal("hot"),
				"description", provider.StringVal("test policy"),
				"states", provider.ListVal([]provider.Value{
					provider.MapVal(buildMap(
						"name", provider.StringVal("hot"),
						"transitions", provider.ListVal([]provider.Value{
							provider.MapVal(buildMap(
								"conditions", provider.MapVal(buildMap(
									"min_index_age", provider.StringVal("7d"),
								)),
								"state_name", provider.StringVal("delete"),
							)),
						}),
					)),
					provider.MapVal(buildMap(
						"actions", provider.ListVal([]provider.Value{
							provider.MapVal(buildMap(
								"delete", provider.MapVal(provider.NewOrderedMap()),
							)),
						}),
						"name", provider.StringVal("delete"),
					)),
				}),
			),
		}

		if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate failed: %v", err)
		}
		requireResourceExists(t, client, "opensearch_ism_policy", policyName)
	})

	t.Run("discover_after_create", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == policyName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("expected to find policy %q in discovered resources", policyName)
		}

		// Verify expected fields present.
		if _, ok := found.Body.Get("states"); !ok {
			t.Error("discovered policy missing states")
		}
		if _, ok := found.Body.Get("default_state"); !ok {
			t.Error("discovered policy missing default_state")
		}
		if _, ok := found.Body.Get("description"); !ok {
			t.Error("discovered policy missing description")
		}

		// Verify metadata is stripped.
		for _, key := range []string{"policy_id", "last_updated_time", "schema_version"} {
			if _, ok := found.Body.Get(key); ok {
				t.Errorf("discovered policy should not have %q", key)
			}
		}

		// error_notification (null) should be stripped.
		if _, ok := found.Body.Get("error_notification"); ok {
			t.Error("discovered policy should not have null error_notification")
		}
	})

	t.Run("normalize_roundtrip", func(t *testing.T) {
		// Discover and find our policy.
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}
		var discovered provider.Resource
		for _, r := range resources {
			if r.ID.Name == policyName {
				discovered = r
				break
			}
		}

		normalizedAPI, err := h.Normalize(ctx, discovered)
		if err != nil {
			t.Fatalf("Normalize discovered resource failed: %v", err)
		}

		// Build DCL resource matching the normalized discovered body.
		// Use the normalized API body as the source of truth for what keys/structure
		// the API returns, so we build an equivalent DCL resource.
		dclResource := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_ism_policy", Name: policyName},
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
		// Change the transition condition from 7d to 14d.
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: policyName},
			Body: buildMap(
				"default_state", provider.StringVal("hot"),
				"description", provider.StringVal("test policy"),
				"states", provider.ListVal([]provider.Value{
					provider.MapVal(buildMap(
						"name", provider.StringVal("hot"),
						"transitions", provider.ListVal([]provider.Value{
							provider.MapVal(buildMap(
								"conditions", provider.MapVal(buildMap(
									"min_index_age", provider.StringVal("14d"),
								)),
								"state_name", provider.StringVal("delete"),
							)),
						}),
					)),
					provider.MapVal(buildMap(
						"actions", provider.ListVal([]provider.Value{
							provider.MapVal(buildMap(
								"delete", provider.MapVal(provider.NewOrderedMap()),
							)),
						}),
						"name", provider.StringVal("delete"),
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
			if resources[i].ID.Name == policyName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("policy %q not found after update", policyName)
		}

		// Verify the transition condition was updated.
		states, _ := found.Body.Get("states")
		hotState := states.List[0].Map
		transitions, _ := hotState.Get("transitions")
		conditions := transitions.List[0].Map
		conds, _ := conditions.Get("conditions")
		age, _ := conds.Map.Get("min_index_age")
		if age.Str != "14d" {
			t.Errorf("expected min_index_age to be 14d after update, got %q", age.Str)
		}
	})

	t.Run("delete", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_ism_policy", Name: policyName},
			Body: provider.NewOrderedMap(),
		}

		if err := h.Apply(ctx, client, provider.OpDelete, r); err != nil {
			t.Fatalf("Apply OpDelete failed: %v", err)
		}
		requireResourceNotExists(t, client, "opensearch_ism_policy", policyName)
	})
}
