package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// --- Unit tests (no cluster needed) ---

func TestClusterSettingsNormalize_idempotent(t *testing.T) {
	h := &clusterSettingsHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_cluster_settings", Name: "cluster"},
		Body: buildMap(
			"action.auto_create_index", provider.StringVal("true"),
			"cluster.routing.allocation.enable", provider.StringVal("all"),
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

func TestClusterSettingsValidate_valid_settings(t *testing.T) {
	h := &clusterSettingsHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_cluster_settings", Name: "cluster"},
		Body: buildMap(
			"action.auto_create_index", provider.StringVal("true"),
		),
	}

	if err := h.Validate(context.Background(), r); err != nil {
		t.Errorf("expected valid settings to pass, got: %v", err)
	}
}

func TestClusterSettingsValidate_non_string_value(t *testing.T) {
	h := &clusterSettingsHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_cluster_settings", Name: "cluster"},
		Body: buildMap(
			"action.auto_create_index", provider.IntVal(1),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-string setting value")
	}
}

// --- Integration tests ---

func TestClusterSettingsHandler_Integration(t *testing.T) {
	client := newTestClient(t)
	h := &clusterSettingsHandler{}
	ctx := context.Background()

	// The setting we'll manage for testing.
	testKey := "action.auto_create_index"

	// Cleanup: reset the test setting after the test.
	t.Cleanup(func() {
		settings := map[string]any{"persistent": map[string]any{testKey: nil}}
		data, _ := json.Marshal(settings)
		req, _ := http.NewRequest(http.MethodPut, "/_cluster/settings", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.api.Client.Perform(req)
		if err != nil {
			t.Logf("cleanup: PUT /_cluster/settings failed: %v", err)
			return
		}
		resp.Body.Close()
	})

	t.Run("create", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_cluster_settings", Name: clusterSettingsSingletonName},
			Body: buildMap(
				testKey, provider.StringVal("false"),
			),
		}

		if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate failed: %v", err)
		}
	})

	t.Run("discover_after_create", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == clusterSettingsSingletonName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatal("expected to find cluster settings in discovered resources")
		}

		v, ok := found.Body.Get(testKey)
		if !ok {
			t.Fatalf("discovered settings missing %q", testKey)
		}
		if v.Str != "false" {
			t.Errorf("expected %q to be \"false\", got %q", testKey, v.Str)
		}
	})

	t.Run("update", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_cluster_settings", Name: clusterSettingsSingletonName},
			Body: buildMap(
				testKey, provider.StringVal("true"),
			),
		}

		if err := h.Apply(ctx, client, provider.OpUpdate, r); err != nil {
			t.Fatalf("Apply OpUpdate failed: %v", err)
		}

		// Verify via Discover.
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover after update failed: %v", err)
		}
		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == clusterSettingsSingletonName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatal("cluster settings not found after update")
		}

		v, _ := found.Body.Get(testKey)
		if v.Str != "true" {
			t.Errorf("expected %q to be \"true\" after update, got %q", testKey, v.Str)
		}
	})

	t.Run("delete_resets_to_default", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_cluster_settings", Name: clusterSettingsSingletonName},
			Body: buildMap(
				testKey, provider.StringVal("true"),
			),
		}

		if err := h.Apply(ctx, client, provider.OpDelete, r); err != nil {
			t.Fatalf("Apply OpDelete failed: %v", err)
		}

		// After delete, the key should no longer appear in persistent settings.
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover after delete failed: %v", err)
		}

		// Either no resource discovered (no persistent settings left),
		// or the test key is absent from the body.
		for _, res := range resources {
			if res.ID.Name == clusterSettingsSingletonName {
				if _, ok := res.Body.Get(testKey); ok {
					t.Errorf("expected %q to be reset after delete, but it still appears in persistent settings", testKey)
				}
			}
		}
	})
}
