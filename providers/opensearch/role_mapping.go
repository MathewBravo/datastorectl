package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MathewBravo/datastorectl/provider"
)

// roleMappingHandler implements resourceHandler for opensearch_role_mapping resources.
type roleMappingHandler struct{}

// Discover fetches all non-reserved, non-hidden, non-static role mappings from OpenSearch.
func (h *roleMappingHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_plugins/_security/api/rolesmapping/", nil)
	if err != nil {
		return nil, fmt.Errorf("opensearch_role_mapping: discover: %s", err)
	}

	body, status, err := client.do(req)
	if err != nil {
		return nil, fmt.Errorf("opensearch_role_mapping: discover: %s", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("opensearch_role_mapping: discover failed (%d): %s", status, body)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("opensearch_role_mapping: discover: %s", err)
	}

	var resources []provider.Resource
	for name, data := range raw {
		var mappingData map[string]any
		if err := json.Unmarshal(data, &mappingData); err != nil {
			return nil, fmt.Errorf("opensearch_role_mapping: discover: failed to decode mapping %q: %s", name, err)
		}

		// Filter out reserved, hidden, and static mappings.
		if isTruthy(mappingData, "reserved") || isTruthy(mappingData, "hidden") || isTruthy(mappingData, "static") {
			continue
		}

		// Strip metadata keys.
		delete(mappingData, "reserved")
		delete(mappingData, "hidden")
		delete(mappingData, "static")

		// Strip empty defaults.
		stripEmptyListField(mappingData, "backend_roles")
		stripEmptyListField(mappingData, "hosts")
		stripEmptyListField(mappingData, "users")
		stripEmptyListField(mappingData, "and_backend_roles")
		stripEmptyStringField(mappingData, "description")

		val := jsonToValue(mappingData)
		resources = append(resources, provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_role_mapping", Name: name},
			Body: val.Map,
		})
	}
	return resources, nil
}

// Normalize sorts set-typed fields and strips empty server defaults.
func (h *roleMappingHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	body := r.Body.Clone()

	// Sort list fields.
	if v, ok := body.Get("backend_roles"); ok {
		body.Set("backend_roles", sortStringList(v))
	}
	if v, ok := body.Get("hosts"); ok {
		body.Set("hosts", sortStringList(v))
	}
	if v, ok := body.Get("users"); ok {
		body.Set("users", sortStringList(v))
	}
	if v, ok := body.Get("and_backend_roles"); ok {
		body.Set("and_backend_roles", sortStringList(v))
	}

	// Strip empty defaults.
	stripEmptyValueList(body, "backend_roles")
	stripEmptyValueList(body, "hosts")
	stripEmptyValueList(body, "users")
	stripEmptyValueList(body, "and_backend_roles")
	stripEmptyValueString(body, "description")

	return provider.Resource{ID: r.ID, Body: body, SourceRange: r.SourceRange}, nil
}

// Validate checks structural correctness of a role mapping resource.
func (h *roleMappingHandler) Validate(_ context.Context, r provider.Resource) error {
	allowed := map[string]bool{
		"backend_roles":     true,
		"hosts":             true,
		"users":             true,
		"and_backend_roles": true,
		"description":       true,
	}

	prefix := fmt.Sprintf("opensearch_role_mapping.%s", r.ID.Name)

	for _, key := range r.Body.Keys() {
		if !allowed[key] {
			return fmt.Errorf("%s: unknown attribute %q (allowed: backend_roles, hosts, users, and_backend_roles, description)", prefix, key)
		}
	}

	// backend_roles — optional list of strings.
	if err := optionalStringList(prefix, r.Body, "backend_roles"); err != nil {
		return err
	}

	// hosts — optional list of strings.
	if err := optionalStringList(prefix, r.Body, "hosts"); err != nil {
		return err
	}

	// users — optional list of strings.
	if err := optionalStringList(prefix, r.Body, "users"); err != nil {
		return err
	}

	// and_backend_roles — optional list of strings.
	if err := optionalStringList(prefix, r.Body, "and_backend_roles"); err != nil {
		return err
	}

	// description — optional string.
	if v, ok := r.Body.Get("description"); ok && v.Kind != provider.KindString {
		return fmt.Errorf("%s: description must be a string, got %s", prefix, v.Kind)
	}

	// TODO: Consider warning when all three of backend_roles, users, hosts are absent/empty.
	// The handler interface returns error, not dcl.Diagnostics, so we cannot
	// emit a warning today.

	return nil
}

// Apply creates, updates, or deletes a role mapping in OpenSearch.
func (h *roleMappingHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate, provider.OpUpdate:
		payload := valueToJSON(provider.MapVal(r.Body))
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("opensearch_role_mapping.%s: %s failed: %s", r.ID.Name, op, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			"/_plugins/_security/api/rolesmapping/"+r.ID.Name,
			bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("opensearch_role_mapping.%s: %s failed: %s", r.ID.Name, op, err)
		}
		req.Header.Set("Content-Type", "application/json")

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_role_mapping.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_role_mapping.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	case provider.OpDelete:
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			"/_plugins/_security/api/rolesmapping/"+r.ID.Name, nil)
		if err != nil {
			return fmt.Errorf("opensearch_role_mapping.%s: %s failed: %s", r.ID.Name, op, err)
		}

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_role_mapping.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status == http.StatusNotFound {
			return nil // already gone
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_role_mapping.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	default:
		return fmt.Errorf("opensearch_role_mapping.%s: unsupported operation %s", r.ID.Name, op)
	}
}
