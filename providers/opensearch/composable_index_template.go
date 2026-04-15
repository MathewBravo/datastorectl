package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/MathewBravo/datastorectl/provider"
)

// composableIndexTemplateHandler implements resourceHandler for opensearch_composable_index_template resources.
type composableIndexTemplateHandler struct{}

// Schema declares that template is always a map block.
func (h *composableIndexTemplateHandler) Schema() provider.Schema {
	return provider.Schema{
		Fields: map[string]provider.FieldHint{
			"template": provider.FieldBlockMap,
		},
	}
}

// Discover fetches all non-system composable index templates from OpenSearch.
func (h *composableIndexTemplateHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_index_template", nil)
	if err != nil {
		return nil, fmt.Errorf("opensearch_composable_index_template: discover: %s", err)
	}

	body, status, err := client.do(req)
	if err != nil {
		return nil, fmt.Errorf("opensearch_composable_index_template: discover: %s", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("opensearch_composable_index_template: discover failed (%d): %s", status, body)
	}

	var resp indexTemplateListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("opensearch_composable_index_template: discover: %s", err)
	}

	var resources []provider.Resource
	for _, entry := range resp.IndexTemplates {
		// Filter system templates (dot-prefixed names are OpenSearch internal).
		if strings.HasPrefix(entry.Name, ".") {
			continue
		}

		templateData := entry.IndexTemplate

		// Strip version — treated as metadata, not desired state.
		delete(templateData, "version")

		// Strip empty defaults.
		stripEmptyListField(templateData, "composed_of")

		val := jsonToValue(templateData)
		resources = append(resources, provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_composable_index_template", Name: entry.Name},
			Body: val.Map,
		})
	}
	return resources, nil
}

// indexTemplateListResponse is the envelope returned by GET /_index_template.
type indexTemplateListResponse struct {
	IndexTemplates []struct {
		Name          string         `json:"name"`
		IndexTemplate map[string]any `json:"index_template"`
	} `json:"index_templates"`
}

// Normalize strips the version field and sorts set-typed lists.
func (h *composableIndexTemplateHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	body := r.Body.Clone()

	// Strip version — not part of desired state.
	body.Delete("version")

	// Sort list fields where order doesn't affect behavior.
	if v, ok := body.Get("index_patterns"); ok {
		body.Set("index_patterns", sortStringList(v))
	}
	if v, ok := body.Get("composed_of"); ok {
		body.Set("composed_of", sortStringList(v))
	}

	// Strip empty defaults.
	stripEmptyValueList(body, "composed_of")

	return provider.Resource{ID: r.ID, Body: body, SourceRange: r.SourceRange}, nil
}

// Validate checks structural correctness of a composable index template resource.
func (h *composableIndexTemplateHandler) Validate(_ context.Context, r provider.Resource) error {
	prefix := fmt.Sprintf("opensearch_composable_index_template.%s", r.ID.Name)

	// index_patterns — required, non-empty list of strings.
	if err := requireStringList(prefix, r.Body, "index_patterns"); err != nil {
		return err
	}
	patternsVal, _ := r.Body.Get("index_patterns")
	if len(patternsVal.List) == 0 {
		return fmt.Errorf("%s: \"index_patterns\" must contain at least one pattern", prefix)
	}

	// composed_of — optional list of strings.
	if err := optionalStringList(prefix, r.Body, "composed_of"); err != nil {
		return err
	}

	// template — optional map.
	if v, ok := r.Body.Get("template"); ok && v.Kind != provider.KindMap {
		return fmt.Errorf("%s: template must be a map, got %s", prefix, v.Kind)
	}

	// priority — optional integer.
	if v, ok := r.Body.Get("priority"); ok && v.Kind != provider.KindInt {
		return fmt.Errorf("%s: priority must be an integer, got %s", prefix, v.Kind)
	}

	return nil
}

// Apply creates, updates, or deletes a composable index template in OpenSearch.
func (h *composableIndexTemplateHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate, provider.OpUpdate:
		payload := valueToJSON(provider.MapVal(r.Body))
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("opensearch_composable_index_template.%s: %s failed: %s", r.ID.Name, op, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			"/_index_template/"+r.ID.Name,
			bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("opensearch_composable_index_template.%s: %s failed: %s", r.ID.Name, op, err)
		}
		req.Header.Set("Content-Type", "application/json")

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_composable_index_template.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_composable_index_template.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	case provider.OpDelete:
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			"/_index_template/"+r.ID.Name, nil)
		if err != nil {
			return fmt.Errorf("opensearch_composable_index_template.%s: %s failed: %s", r.ID.Name, op, err)
		}

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_composable_index_template.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status == http.StatusNotFound {
			return nil // already gone
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_composable_index_template.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	default:
		return fmt.Errorf("opensearch_composable_index_template.%s: unsupported operation %s", r.ID.Name, op)
	}
}
