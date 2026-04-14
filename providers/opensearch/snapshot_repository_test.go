package opensearch

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// --- Unit tests (no cluster needed) ---

func TestSnapshotRepositoryNormalize_idempotent(t *testing.T) {
	h := &snapshotRepositoryHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_snapshot_repository", Name: "test"},
		Body: buildMap(
			"settings", provider.MapVal(buildMap(
				"location", provider.StringVal("/mnt/snapshots"),
			)),
			"type", provider.StringVal("fs"),
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

func TestSnapshotRepositoryValidate_valid_repo(t *testing.T) {
	h := &snapshotRepositoryHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_snapshot_repository", Name: "good"},
		Body: buildMap(
			"settings", provider.MapVal(buildMap(
				"location", provider.StringVal("/mnt/snapshots"),
			)),
			"type", provider.StringVal("fs"),
		),
	}

	if err := h.Validate(context.Background(), r); err != nil {
		t.Errorf("expected valid repo to pass, got: %v", err)
	}
}

func TestSnapshotRepositoryValidate_missing_type(t *testing.T) {
	h := &snapshotRepositoryHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_snapshot_repository", Name: "bad"},
		Body: buildMap(
			"settings", provider.MapVal(buildMap(
				"location", provider.StringVal("/mnt/snapshots"),
			)),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestSnapshotRepositoryValidate_type_wrong_type(t *testing.T) {
	h := &snapshotRepositoryHandler{}
	r := provider.Resource{
		ID:   provider.ResourceID{Type: "opensearch_snapshot_repository", Name: "bad"},
		Body: buildMap("type", provider.IntVal(42)),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-string type")
	}
}

func TestSnapshotRepositoryValidate_settings_wrong_type(t *testing.T) {
	h := &snapshotRepositoryHandler{}
	r := provider.Resource{
		ID: provider.ResourceID{Type: "opensearch_snapshot_repository", Name: "bad"},
		Body: buildMap(
			"type", provider.StringVal("fs"),
			"settings", provider.StringVal("not_a_map"),
		),
	}

	err := h.Validate(context.Background(), r)
	if err == nil {
		t.Fatal("expected error for non-map settings")
	}
}

// --- Integration tests ---

func TestSnapshotRepositoryHandler_Integration(t *testing.T) {
	client := newTestClient(t)
	h := &snapshotRepositoryHandler{}
	repoName := "datastorectl_test_repo"
	cleanupResource(t, client, "opensearch_snapshot_repository", repoName)

	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_snapshot_repository", Name: repoName},
			Body: buildMap(
				"settings", provider.MapVal(buildMap(
					"location", provider.StringVal("/usr/share/opensearch/snapshots"),
				)),
				"type", provider.StringVal("fs"),
			),
		}

		if err := h.Apply(ctx, client, provider.OpCreate, r); err != nil {
			t.Fatalf("Apply OpCreate failed: %v", err)
		}
		requireResourceExists(t, client, "opensearch_snapshot_repository", repoName)
	})

	t.Run("discover_after_create", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		var found *provider.Resource
		for i := range resources {
			if resources[i].ID.Name == repoName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("expected to find repository %q in discovered resources", repoName)
		}

		if _, ok := found.Body.Get("type"); !ok {
			t.Error("discovered repo missing type")
		}
		if _, ok := found.Body.Get("settings"); !ok {
			t.Error("discovered repo missing settings")
		}
	})

	t.Run("normalize_roundtrip", func(t *testing.T) {
		resources, err := h.Discover(ctx, client)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}
		var discovered provider.Resource
		for _, r := range resources {
			if r.ID.Name == repoName {
				discovered = r
				break
			}
		}

		normalizedAPI, err := h.Normalize(ctx, discovered)
		if err != nil {
			t.Fatalf("Normalize discovered resource failed: %v", err)
		}

		dclResource := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_snapshot_repository", Name: repoName},
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
		// Change compress setting.
		r := provider.Resource{
			ID: provider.ResourceID{Type: "opensearch_snapshot_repository", Name: repoName},
			Body: buildMap(
				"settings", provider.MapVal(buildMap(
					"compress", provider.StringVal("true"),
					"location", provider.StringVal("/usr/share/opensearch/snapshots"),
				)),
				"type", provider.StringVal("fs"),
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
			if resources[i].ID.Name == repoName {
				found = &resources[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("repository %q not found after update", repoName)
		}

		settings, _ := found.Body.Get("settings")
		compress, ok := settings.Map.Get("compress")
		if !ok {
			t.Fatal("compress setting missing after update")
		}
		if compress.Str != "true" {
			t.Errorf("expected compress to be \"true\", got %q", compress.Str)
		}
	})

	t.Run("delete", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_snapshot_repository", Name: repoName},
			Body: provider.NewOrderedMap(),
		}

		if err := h.Apply(ctx, client, provider.OpDelete, r); err != nil {
			t.Fatalf("Apply OpDelete failed: %v", err)
		}
		requireResourceNotExists(t, client, "opensearch_snapshot_repository", repoName)
	})
}
