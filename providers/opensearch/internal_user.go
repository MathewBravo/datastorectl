package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MathewBravo/datastorectl/provider"
)

// internalUserHandler implements resourceHandler for opensearch_internal_user resources.
type internalUserHandler struct{}

// Discover fetches all non-reserved, non-hidden, non-static internal users from OpenSearch.
func (h *internalUserHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_plugins/_security/api/internalusers/", nil)
	if err != nil {
		return nil, fmt.Errorf("opensearch_internal_user: discover: %s", err)
	}

	body, status, err := client.do(req)
	if err != nil {
		return nil, fmt.Errorf("opensearch_internal_user: discover: %s", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("opensearch_internal_user: discover failed (%d): %s", status, body)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("opensearch_internal_user: discover: %s", err)
	}

	var resources []provider.Resource
	for name, data := range raw {
		var userData map[string]any
		if err := json.Unmarshal(data, &userData); err != nil {
			return nil, fmt.Errorf("opensearch_internal_user: discover: failed to decode user %q: %s", name, err)
		}

		// Filter out reserved, hidden, and static users.
		if isTruthy(userData, "reserved") || isTruthy(userData, "hidden") || isTruthy(userData, "static") {
			continue
		}

		// Strip metadata keys.
		delete(userData, "reserved")
		delete(userData, "hidden")
		delete(userData, "static")

		// Strip server-computed hash (write-only — users specify password or hash in DCL).
		delete(userData, "hash")

		// Strip empty defaults.
		stripEmptyListField(userData, "backend_roles")
		stripEmptyListField(userData, "opendistro_security_roles")
		stripEmptyStringField(userData, "description")
		stripEmptyMapField(userData, "attributes")

		val := jsonToValue(userData)
		resources = append(resources, provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_internal_user", Name: name},
			Body: val.Map,
		})
	}
	return resources, nil
}

// Normalize sorts set-typed fields and strips empty server defaults.
func (h *internalUserHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	body := r.Body.Clone()

	// Sort list fields.
	if v, ok := body.Get("backend_roles"); ok {
		body.Set("backend_roles", sortStringList(v))
	}
	if v, ok := body.Get("opendistro_security_roles"); ok {
		body.Set("opendistro_security_roles", sortStringList(v))
	}

	// Strip empty defaults.
	stripEmptyValueList(body, "backend_roles")
	stripEmptyValueList(body, "opendistro_security_roles")
	stripEmptyValueString(body, "description")
	stripEmptyValueMap(body, "attributes")

	return provider.Resource{ID: r.ID, Body: body, SourceRange: r.SourceRange}, nil
}

// Validate checks structural correctness of an internal user resource.
func (h *internalUserHandler) Validate(_ context.Context, r provider.Resource) error {
	allowed := map[string]bool{
		"password":                  true,
		"hash":                      true,
		"backend_roles":             true,
		"attributes":                true,
		"description":               true,
		"opendistro_security_roles": true,
	}

	prefix := fmt.Sprintf("opensearch_internal_user.%s", r.ID.Name)

	for _, key := range r.Body.Keys() {
		if !allowed[key] {
			return fmt.Errorf("%s: unknown attribute %q (allowed: password, hash, backend_roles, attributes, description, opendistro_security_roles)", prefix, key)
		}
	}

	// password — optional string.
	if v, ok := r.Body.Get("password"); ok && v.Kind != provider.KindString {
		return fmt.Errorf("%s: password must be a string, got %s", prefix, v.Kind)
	}

	// hash — optional string.
	if v, ok := r.Body.Get("hash"); ok && v.Kind != provider.KindString {
		return fmt.Errorf("%s: hash must be a string, got %s", prefix, v.Kind)
	}

	// description — optional string.
	if v, ok := r.Body.Get("description"); ok && v.Kind != provider.KindString {
		return fmt.Errorf("%s: description must be a string, got %s", prefix, v.Kind)
	}

	// backend_roles — optional list of strings.
	if err := optionalStringList(prefix, r.Body, "backend_roles"); err != nil {
		return err
	}

	// opendistro_security_roles — optional list of strings.
	if err := optionalStringList(prefix, r.Body, "opendistro_security_roles"); err != nil {
		return err
	}

	// attributes — optional map with all string values.
	if v, ok := r.Body.Get("attributes"); ok {
		if v.Kind != provider.KindMap {
			return fmt.Errorf("%s: attributes must be a map, got %s", prefix, v.Kind)
		}
		for _, k := range v.Map.Keys() {
			val, _ := v.Map.Get(k)
			if val.Kind != provider.KindString {
				return fmt.Errorf("%s: attributes.%s must be a string, got %s", prefix, k, val.Kind)
			}
		}
	}

	// TODO: Consider warning when neither password nor hash is set.
	// The handler interface returns error, not dcl.Diagnostics, so we cannot
	// emit a warning today. Discovered resources legitimately lack both fields.

	return nil
}

// Apply creates, updates, or deletes an internal user in OpenSearch.
func (h *internalUserHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate, provider.OpUpdate:
		payload := valueToJSON(provider.MapVal(r.Body))
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("opensearch_internal_user.%s: %s failed: %s", r.ID.Name, op, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			"/_plugins/_security/api/internalusers/"+r.ID.Name,
			bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("opensearch_internal_user.%s: %s failed: %s", r.ID.Name, op, err)
		}
		req.Header.Set("Content-Type", "application/json")

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_internal_user.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_internal_user.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	case provider.OpDelete:
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			"/_plugins/_security/api/internalusers/"+r.ID.Name, nil)
		if err != nil {
			return fmt.Errorf("opensearch_internal_user.%s: %s failed: %s", r.ID.Name, op, err)
		}

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_internal_user.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status == http.StatusNotFound {
			return nil // already gone
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_internal_user.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	default:
		return fmt.Errorf("opensearch_internal_user.%s: unsupported operation %s", r.ID.Name, op)
	}
}
