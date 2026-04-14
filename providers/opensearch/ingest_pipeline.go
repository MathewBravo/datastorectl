package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MathewBravo/datastorectl/provider"
)

// ingestPipelineHandler implements resourceHandler for opensearch_ingest_pipeline resources.
type ingestPipelineHandler struct{}

// Discover fetches all ingest pipelines from OpenSearch.
func (h *ingestPipelineHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_ingest/pipeline", nil)
	if err != nil {
		return nil, fmt.Errorf("opensearch_ingest_pipeline: discover: %s", err)
	}

	body, status, err := client.do(req)
	if err != nil {
		return nil, fmt.Errorf("opensearch_ingest_pipeline: discover: %s", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("opensearch_ingest_pipeline: discover failed (%d): %s", status, body)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("opensearch_ingest_pipeline: discover: %s", err)
	}

	var resources []provider.Resource
	for name, data := range raw {
		var pipelineData map[string]any
		if err := json.Unmarshal(data, &pipelineData); err != nil {
			return nil, fmt.Errorf("opensearch_ingest_pipeline: discover: failed to decode pipeline %q: %s", name, err)
		}

		// Strip empty defaults.
		stripEmptyStringField(pipelineData, "description")

		val := jsonToValue(pipelineData)
		resources = append(resources, provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_ingest_pipeline", Name: name},
			Body: val.Map,
		})
	}
	return resources, nil
}

// Normalize strips empty defaults. Processor order is preserved — sequence matters.
func (h *ingestPipelineHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	body := r.Body.Clone()

	stripEmptyValueString(body, "description")

	return provider.Resource{ID: r.ID, Body: body, SourceRange: r.SourceRange}, nil
}

// Validate checks structural correctness of an ingest pipeline resource.
func (h *ingestPipelineHandler) Validate(_ context.Context, r provider.Resource) error {
	prefix := fmt.Sprintf("opensearch_ingest_pipeline.%s", r.ID.Name)

	// processors — required list.
	processorsVal, ok := r.Body.Get("processors")
	if !ok {
		return fmt.Errorf("%s: \"processors\" is required — an ingest pipeline must define at least one processor", prefix)
	}
	if processorsVal.Kind != provider.KindList {
		return fmt.Errorf("%s: processors must be a list, got %s", prefix, processorsVal.Kind)
	}

	// description — optional string.
	if v, ok := r.Body.Get("description"); ok && v.Kind != provider.KindString {
		return fmt.Errorf("%s: description must be a string, got %s", prefix, v.Kind)
	}

	return nil
}

// Apply creates, updates, or deletes an ingest pipeline in OpenSearch.
func (h *ingestPipelineHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate, provider.OpUpdate:
		payload := valueToJSON(provider.MapVal(r.Body))
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("opensearch_ingest_pipeline.%s: %s failed: %s", r.ID.Name, op, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			"/_ingest/pipeline/"+r.ID.Name,
			bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("opensearch_ingest_pipeline.%s: %s failed: %s", r.ID.Name, op, err)
		}
		req.Header.Set("Content-Type", "application/json")

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_ingest_pipeline.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_ingest_pipeline.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	case provider.OpDelete:
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			"/_ingest/pipeline/"+r.ID.Name, nil)
		if err != nil {
			return fmt.Errorf("opensearch_ingest_pipeline.%s: %s failed: %s", r.ID.Name, op, err)
		}

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_ingest_pipeline.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status == http.StatusNotFound {
			return nil // already gone
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_ingest_pipeline.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	default:
		return fmt.Errorf("opensearch_ingest_pipeline.%s: unsupported operation %s", r.ID.Name, op)
	}
}
