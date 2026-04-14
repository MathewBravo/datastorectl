package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MathewBravo/datastorectl/provider"
)

// snapshotRepositoryHandler implements resourceHandler for opensearch_snapshot_repository resources.
type snapshotRepositoryHandler struct{}

// Discover fetches all snapshot repositories from OpenSearch.
func (h *snapshotRepositoryHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_snapshot/_all", nil)
	if err != nil {
		return nil, fmt.Errorf("opensearch_snapshot_repository: discover: %s", err)
	}

	body, status, err := client.do(req)
	if err != nil {
		return nil, fmt.Errorf("opensearch_snapshot_repository: discover: %s", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("opensearch_snapshot_repository: discover failed (%d): %s", status, body)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("opensearch_snapshot_repository: discover: %s", err)
	}

	var resources []provider.Resource
	for name, data := range raw {
		var repoData map[string]any
		if err := json.Unmarshal(data, &repoData); err != nil {
			return nil, fmt.Errorf("opensearch_snapshot_repository: discover: failed to decode repository %q: %s", name, err)
		}

		val := jsonToValue(repoData)
		resources = append(resources, provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_snapshot_repository", Name: name},
			Body: val.Map,
		})
	}
	return resources, nil
}

// Normalize is a no-op — settings keys are already sorted by jsonToValue during Discover.
func (h *snapshotRepositoryHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	body := r.Body.Clone()
	return provider.Resource{ID: r.ID, Body: body, SourceRange: r.SourceRange}, nil
}

// Validate checks structural correctness of a snapshot repository resource.
func (h *snapshotRepositoryHandler) Validate(_ context.Context, r provider.Resource) error {
	prefix := fmt.Sprintf("opensearch_snapshot_repository.%s", r.ID.Name)

	// type — required string.
	typeVal, ok := r.Body.Get("type")
	if !ok {
		return fmt.Errorf("%s: \"type\" is required — specify the repository type (e.g. \"fs\", \"s3\")", prefix)
	}
	if typeVal.Kind != provider.KindString {
		return fmt.Errorf("%s: type must be a string, got %s", prefix, typeVal.Kind)
	}

	// settings — optional map.
	if v, ok := r.Body.Get("settings"); ok && v.Kind != provider.KindMap {
		return fmt.Errorf("%s: settings must be a map, got %s", prefix, v.Kind)
	}

	return nil
}

// Apply creates, updates, or deletes a snapshot repository in OpenSearch.
func (h *snapshotRepositoryHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate, provider.OpUpdate:
		payload := valueToJSON(provider.MapVal(r.Body))
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("opensearch_snapshot_repository.%s: %s failed: %s", r.ID.Name, op, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			"/_snapshot/"+r.ID.Name,
			bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("opensearch_snapshot_repository.%s: %s failed: %s", r.ID.Name, op, err)
		}
		req.Header.Set("Content-Type", "application/json")

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_snapshot_repository.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_snapshot_repository.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	case provider.OpDelete:
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			"/_snapshot/"+r.ID.Name, nil)
		if err != nil {
			return fmt.Errorf("opensearch_snapshot_repository.%s: %s failed: %s", r.ID.Name, op, err)
		}

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_snapshot_repository.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status == http.StatusNotFound {
			return nil // already gone
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_snapshot_repository.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	default:
		return fmt.Errorf("opensearch_snapshot_repository.%s: unsupported operation %s", r.ID.Name, op)
	}
}
