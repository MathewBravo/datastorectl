package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MathewBravo/datastorectl/provider"
)

// roleHandler implements resourceHandler for opensearch_role resources.
type roleHandler struct{}

// Discover fetches all non-reserved, non-hidden, non-static roles from OpenSearch.
func (h *roleHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_plugins/_security/api/roles/", nil)
	if err != nil {
		return nil, fmt.Errorf("opensearch_role: discover: %s", err)
	}

	body, status, err := client.do(req)
	if err != nil {
		return nil, fmt.Errorf("opensearch_role: discover: %s", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("opensearch_role: discover failed (%d): %s", status, body)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("opensearch_role: discover: %s", err)
	}

	var resources []provider.Resource
	for name, data := range raw {
		var roleData map[string]any
		if err := json.Unmarshal(data, &roleData); err != nil {
			return nil, fmt.Errorf("opensearch_role: discover: failed to decode role %q: %s", name, err)
		}

		// Filter out reserved, hidden, and static roles.
		if isTruthy(roleData, "reserved") || isTruthy(roleData, "hidden") || isTruthy(roleData, "static") {
			continue
		}

		// Strip metadata keys.
		delete(roleData, "reserved")
		delete(roleData, "hidden")
		delete(roleData, "static")

		// Strip server defaults within index_permissions entries.
		if ips, ok := roleData["index_permissions"].([]any); ok {
			for _, entry := range ips {
				if m, ok := entry.(map[string]any); ok {
					stripEmptyListField(m, "fls")
					stripEmptyListField(m, "masked_fields")
					stripEmptyStringField(m, "dls")
				}
			}
		}

		// Strip top-level empty lists.
		stripEmptyListField(roleData, "cluster_permissions")
		stripEmptyListField(roleData, "index_permissions")
		stripEmptyListField(roleData, "tenant_permissions")

		val := jsonToValue(roleData)
		resources = append(resources, provider.Resource{
			ID:   provider.ResourceID{Type: "opensearch_role", Name: name},
			Body: val.Map,
		})
	}
	return resources, nil
}

// Normalize sorts set-typed fields and strips empty server defaults.
func (h *roleHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	body := r.Body.Clone()

	// Sort top-level cluster_permissions.
	if v, ok := body.Get("cluster_permissions"); ok {
		body.Set("cluster_permissions", sortStringList(v))
	}

	// Sort fields within index_permissions entries.
	if v, ok := body.Get("index_permissions"); ok && v.Kind == provider.KindList {
		for i, entry := range v.List {
			if entry.Kind != provider.KindMap {
				continue
			}
			m := entry.Map
			for _, field := range []string{"index_patterns", "allowed_actions", "fls", "masked_fields"} {
				if fv, ok := m.Get(field); ok {
					m.Set(field, sortStringList(fv))
				}
			}
			// Strip empty defaults within entries.
			stripEmptyValueList(m, "fls")
			stripEmptyValueList(m, "masked_fields")
			stripEmptyValueString(m, "dls")
			v.List[i] = provider.MapVal(m)
		}
		body.Set("index_permissions", v)
	}

	// Sort fields within tenant_permissions entries.
	if v, ok := body.Get("tenant_permissions"); ok && v.Kind == provider.KindList {
		for i, entry := range v.List {
			if entry.Kind != provider.KindMap {
				continue
			}
			m := entry.Map
			for _, field := range []string{"tenant_patterns", "allowed_actions"} {
				if fv, ok := m.Get(field); ok {
					m.Set(field, sortStringList(fv))
				}
			}
			v.List[i] = provider.MapVal(m)
		}
		body.Set("tenant_permissions", v)
	}

	// Strip empty top-level lists.
	stripEmptyValueList(body, "cluster_permissions")
	stripEmptyValueList(body, "index_permissions")
	stripEmptyValueList(body, "tenant_permissions")

	return provider.Resource{ID: r.ID, Body: body, SourceRange: r.SourceRange}, nil
}

// Validate checks structural correctness of a role resource.
func (h *roleHandler) Validate(_ context.Context, r provider.Resource) error {
	allowed := map[string]bool{
		"cluster_permissions": true,
		"index_permissions":   true,
		"tenant_permissions":  true,
	}

	for _, key := range r.Body.Keys() {
		if !allowed[key] {
			return fmt.Errorf("opensearch_role.%s: unknown attribute %q (allowed: cluster_permissions, index_permissions, tenant_permissions)", r.ID.Name, key)
		}
	}

	// cluster_permissions
	if v, ok := r.Body.Get("cluster_permissions"); ok {
		if v.Kind != provider.KindList {
			return fmt.Errorf("opensearch_role.%s: cluster_permissions must be a list, got %s", r.ID.Name, v.Kind)
		}
		for i, elem := range v.List {
			if elem.Kind != provider.KindString {
				return fmt.Errorf("opensearch_role.%s: cluster_permissions[%d] must be a string, got %s", r.ID.Name, i, elem.Kind)
			}
		}
	}

	// index_permissions
	if v, ok := r.Body.Get("index_permissions"); ok {
		if v.Kind != provider.KindList {
			return fmt.Errorf("opensearch_role.%s: index_permissions must be a list, got %s", r.ID.Name, v.Kind)
		}
		for i, elem := range v.List {
			if elem.Kind != provider.KindMap {
				return fmt.Errorf("opensearch_role.%s: index_permissions[%d] must be a map, got %s", r.ID.Name, i, elem.Kind)
			}
			if err := validateIndexPermission(r.ID.Name, i, elem.Map); err != nil {
				return err
			}
		}
	}

	// tenant_permissions
	if v, ok := r.Body.Get("tenant_permissions"); ok {
		if v.Kind != provider.KindList {
			return fmt.Errorf("opensearch_role.%s: tenant_permissions must be a list, got %s", r.ID.Name, v.Kind)
		}
		for i, elem := range v.List {
			if elem.Kind != provider.KindMap {
				return fmt.Errorf("opensearch_role.%s: tenant_permissions[%d] must be a map, got %s", r.ID.Name, i, elem.Kind)
			}
			if err := validateTenantPermission(r.ID.Name, i, elem.Map); err != nil {
				return err
			}
		}
	}

	return nil
}

// Apply creates, updates, or deletes a role in OpenSearch.
func (h *roleHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate, provider.OpUpdate:
		payload := valueToJSON(provider.MapVal(r.Body))
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("opensearch_role.%s: %s failed: %s", r.ID.Name, op, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut,
			"/_plugins/_security/api/roles/"+r.ID.Name,
			bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("opensearch_role.%s: %s failed: %s", r.ID.Name, op, err)
		}
		req.Header.Set("Content-Type", "application/json")

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_role.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_role.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	case provider.OpDelete:
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			"/_plugins/_security/api/roles/"+r.ID.Name, nil)
		if err != nil {
			return fmt.Errorf("opensearch_role.%s: %s failed: %s", r.ID.Name, op, err)
		}

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_role.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status == http.StatusNotFound {
			return nil // already gone
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_role.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	default:
		return fmt.Errorf("opensearch_role.%s: unsupported operation %s", r.ID.Name, op)
	}
}

// --- validation helpers ---

func validateIndexPermission(roleName string, idx int, m *provider.OrderedMap) error {
	prefix := fmt.Sprintf("opensearch_role.%s: index_permissions[%d]", roleName, idx)

	if err := requireStringList(prefix, m, "index_patterns"); err != nil {
		return err
	}
	if err := requireStringList(prefix, m, "allowed_actions"); err != nil {
		return err
	}
	if err := optionalStringList(prefix, m, "fls"); err != nil {
		return err
	}
	if err := optionalStringList(prefix, m, "masked_fields"); err != nil {
		return err
	}
	if v, ok := m.Get("dls"); ok && v.Kind != provider.KindString {
		return fmt.Errorf("%s.dls must be a string, got %s", prefix, v.Kind)
	}
	return nil
}

func validateTenantPermission(roleName string, idx int, m *provider.OrderedMap) error {
	prefix := fmt.Sprintf("opensearch_role.%s: tenant_permissions[%d]", roleName, idx)

	if err := requireStringList(prefix, m, "tenant_patterns"); err != nil {
		return err
	}
	if err := requireStringList(prefix, m, "allowed_actions"); err != nil {
		return err
	}
	return nil
}


