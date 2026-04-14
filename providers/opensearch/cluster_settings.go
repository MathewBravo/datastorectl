package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MathewBravo/datastorectl/provider"
)

// clusterSettingsHandler implements resourceHandler for opensearch_cluster_settings resources.
// This is a singleton resource — one per cluster with canonical name "cluster".
type clusterSettingsHandler struct{}

// clusterSettingsSingletonName is the canonical resource name for the cluster settings singleton.
const clusterSettingsSingletonName = "cluster"

// Discover fetches persistent cluster settings from OpenSearch.
// Returns at most one resource. Returns an empty list if no persistent settings are configured.
func (h *clusterSettingsHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_cluster/settings?flat_settings=true", nil)
	if err != nil {
		return nil, fmt.Errorf("opensearch_cluster_settings: discover: %s", err)
	}

	body, status, err := client.do(req)
	if err != nil {
		return nil, fmt.Errorf("opensearch_cluster_settings: discover: %s", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("opensearch_cluster_settings: discover failed (%d): %s", status, body)
	}

	var resp struct {
		Persistent map[string]any `json:"persistent"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("opensearch_cluster_settings: discover: %s", err)
	}

	// No persistent settings configured — nothing to discover.
	if len(resp.Persistent) == 0 {
		return nil, nil
	}

	val := jsonToValue(resp.Persistent)
	return []provider.Resource{{
		ID:   provider.ResourceID{Type: "opensearch_cluster_settings", Name: clusterSettingsSingletonName},
		Body: val.Map,
	}}, nil
}

// Normalize is minimal — flat settings are already in canonical form.
func (h *clusterSettingsHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	body := r.Body.Clone()
	return provider.Resource{ID: r.ID, Body: body, SourceRange: r.SourceRange}, nil
}

// Validate checks structural correctness of a cluster settings resource.
func (h *clusterSettingsHandler) Validate(_ context.Context, r provider.Resource) error {
	prefix := fmt.Sprintf("opensearch_cluster_settings.%s", r.ID.Name)

	// All values must be strings (flat settings are string key-value pairs).
	for _, key := range r.Body.Keys() {
		v, _ := r.Body.Get(key)
		if v.Kind != provider.KindString {
			return fmt.Errorf("%s: setting %q must be a string, got %s", prefix, key, v.Kind)
		}
	}

	// TODO: Reject more than one opensearch_cluster_settings block.
	// The handler interface validates one resource at a time, so this check
	// would need to happen at the engine level.

	return nil
}

// Apply creates, updates, or deletes (resets) cluster settings.
// Create and Update send a partial PUT with only the user-declared keys.
// Delete resets each managed key to its default by PUTting null values.
func (h *clusterSettingsHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate, provider.OpUpdate:
		settings := make(map[string]any)
		for _, key := range r.Body.Keys() {
			v, _ := r.Body.Get(key)
			settings[key] = valueToJSON(v)
		}

		envelope := map[string]any{"persistent": settings}
		data, err := json.Marshal(envelope)
		if err != nil {
			return fmt.Errorf("opensearch_cluster_settings.%s: %s failed: %s", r.ID.Name, op, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			"/_cluster/settings", bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("opensearch_cluster_settings.%s: %s failed: %s", r.ID.Name, op, err)
		}
		req.Header.Set("Content-Type", "application/json")

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_cluster_settings.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_cluster_settings.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	case provider.OpDelete:
		// Reset each managed key to default by sending null.
		settings := make(map[string]any)
		for _, key := range r.Body.Keys() {
			settings[key] = nil
		}

		envelope := map[string]any{"persistent": settings}
		data, err := json.Marshal(envelope)
		if err != nil {
			return fmt.Errorf("opensearch_cluster_settings.%s: %s failed: %s", r.ID.Name, op, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			"/_cluster/settings", bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("opensearch_cluster_settings.%s: %s failed: %s", r.ID.Name, op, err)
		}
		req.Header.Set("Content-Type", "application/json")

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_cluster_settings.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_cluster_settings.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	default:
		return fmt.Errorf("opensearch_cluster_settings.%s: unsupported operation %s", r.ID.Name, op)
	}
}
