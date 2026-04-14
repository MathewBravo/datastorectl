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

// componentTemplateHandler implements resourceHandler for opensearch_component_template resources.
type componentTemplateHandler struct{}

// Discover fetches all non-system component templates from OpenSearch.
func (h *componentTemplateHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_component_template", nil)
	if err != nil {
		return nil, fmt.Errorf("opensearch_component_template: discover: %s", err)
	}

	body, status, err := client.do(req)
	if err != nil {
		return nil, fmt.Errorf("opensearch_component_template: discover: %s", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("opensearch_component_template: discover failed (%d): %s", status, body)
	}

	var resp componentTemplateListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("opensearch_component_template: discover: %s", err)
	}

	var resources []provider.Resource
	for _, entry := range resp.ComponentTemplates {
		// Filter system templates (dot-prefixed names are OpenSearch internal).
		if strings.HasPrefix(entry.Name, ".") {
			continue
		}

		templateData := entry.ComponentTemplate

		// Strip version — treated as metadata, not desired state.
		delete(templateData, "version")

		val := jsonToValue(templateData)
		resources = append(resources, provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_component_template", Name: entry.Name},
			Body: val.Map,
		})
	}
	return resources, nil
}

// componentTemplateListResponse is the envelope returned by GET /_component_template.
type componentTemplateListResponse struct {
	ComponentTemplates []struct {
		Name              string         `json:"name"`
		ComponentTemplate map[string]any `json:"component_template"`
	} `json:"component_templates"`
}

// Normalize strips the version field. Map keys are already sorted deterministically
// by jsonToValue during Discover.
func (h *componentTemplateHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	body := r.Body.Clone()

	// Strip version — not part of desired state.
	body.Delete("version")

	return provider.Resource{ID: r.ID, Body: body, SourceRange: r.SourceRange}, nil
}

// Validate checks structural correctness of a component template resource.
func (h *componentTemplateHandler) Validate(_ context.Context, r provider.Resource) error {
	prefix := fmt.Sprintf("opensearch_component_template.%s", r.ID.Name)

	// template — required map.
	tmpl, ok := r.Body.Get("template")
	if !ok {
		return fmt.Errorf("%s: \"template\" is required — a component template must define a template block with settings, mappings, or aliases", prefix)
	}
	if tmpl.Kind != provider.KindMap {
		return fmt.Errorf("%s: template must be a map, got %s", prefix, tmpl.Kind)
	}

	return nil
}

// Apply creates, updates, or deletes a component template in OpenSearch.
func (h *componentTemplateHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate, provider.OpUpdate:
		payload := valueToJSON(provider.MapVal(r.Body))
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("opensearch_component_template.%s: %s failed: %s", r.ID.Name, op, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			"/_component_template/"+r.ID.Name,
			bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("opensearch_component_template.%s: %s failed: %s", r.ID.Name, op, err)
		}
		req.Header.Set("Content-Type", "application/json")

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_component_template.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_component_template.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	case provider.OpDelete:
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			"/_component_template/"+r.ID.Name, nil)
		if err != nil {
			return fmt.Errorf("opensearch_component_template.%s: %s failed: %s", r.ID.Name, op, err)
		}

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_component_template.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status == http.StatusNotFound {
			return nil // already gone
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_component_template.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	default:
		return fmt.Errorf("opensearch_component_template.%s: unsupported operation %s", r.ID.Name, op)
	}
}
