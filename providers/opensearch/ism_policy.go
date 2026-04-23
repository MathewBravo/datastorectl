package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/MathewBravo/datastorectl/provider"
)

const (
	ismDefaultRetryCount   = 3
	ismDefaultRetryBackoff = "exponential"
	ismDefaultRetryDelay   = "1m"
)

// ismPolicyHandler implements resourceHandler for opensearch_ism_policy resources.
type ismPolicyHandler struct{}

// isDefaultRetryJSON reports whether a raw JSON retry map equals the server
// default that OpenSearch injects into every action body. Keeping this check
// centralized makes the "preserve user overrides" contract explicit.
func isDefaultRetryJSON(m map[string]any) bool {
	if len(m) != 3 {
		return false
	}
	count, cok := m["count"].(float64) // JSON numbers decode as float64
	backoff, bok := m["backoff"].(string)
	delay, dok := m["delay"].(string)
	return cok && bok && dok &&
		int(count) == ismDefaultRetryCount &&
		backoff == ismDefaultRetryBackoff &&
		delay == ismDefaultRetryDelay
}

// isDefaultRetryValue is the provider.Value equivalent used during Normalize.
func isDefaultRetryValue(v provider.Value) bool {
	if v.Kind != provider.KindMap || v.Map == nil || v.Map.Len() != 3 {
		return false
	}
	count, cok := v.Map.Get("count")
	backoff, bok := v.Map.Get("backoff")
	delay, dok := v.Map.Get("delay")
	return cok && bok && dok &&
		count.Kind == provider.KindInt && count.Int == ismDefaultRetryCount &&
		backoff.Kind == provider.KindString && backoff.Str == ismDefaultRetryBackoff &&
		delay.Kind == provider.KindString && delay.Str == ismDefaultRetryDelay
}

// Discover fetches all ISM policies from OpenSearch, handling pagination.
func (h *ismPolicyHandler) Discover(ctx context.Context, client *Client) ([]provider.Resource, error) {
	var resources []provider.Resource
	const pageSize = 50

	for from := 0; ; {
		url := fmt.Sprintf("/_plugins/_ism/policies?size=%d&from=%d", pageSize, from)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("opensearch_ism_policy: discover: %s", err)
		}

		body, status, err := client.do(req)
		if err != nil {
			return nil, fmt.Errorf("opensearch_ism_policy: discover: %s", err)
		}
		if status < 200 || status >= 300 {
			return nil, fmt.Errorf("opensearch_ism_policy: discover failed (%d): %s", status, body)
		}

		var resp ismListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("opensearch_ism_policy: discover: %s", err)
		}

		for _, entry := range resp.Policies {
			policyData := entry.Policy

			// Strip server-injected metadata.
			delete(policyData, "policy_id")
			delete(policyData, "last_updated_time")
			delete(policyData, "schema_version")

			// Strip error_notification if null.
			if v, ok := policyData["error_notification"]; ok && v == nil {
				delete(policyData, "error_notification")
			}

			// Strip ism_template if null — semantically identical to absent.
			if v, ok := policyData["ism_template"]; ok && v == nil {
				delete(policyData, "ism_template")
			}

			// Strip default retry from each action body in each state.
			if states, ok := policyData["states"].([]any); ok {
				for _, state := range states {
					sm, ok := state.(map[string]any)
					if !ok {
						continue
					}
					actions, ok := sm["actions"].([]any)
					if !ok {
						continue
					}
					for _, action := range actions {
						am, ok := action.(map[string]any)
						if !ok {
							continue
						}
						if retry, ok := am["retry"].(map[string]any); ok && isDefaultRetryJSON(retry) {
							delete(am, "retry")
						}
					}
				}
			}

			// Strip last_updated_time from ism_template entries.
			if templates, ok := policyData["ism_template"].([]any); ok {
				for _, tmpl := range templates {
					if m, ok := tmpl.(map[string]any); ok {
						delete(m, "last_updated_time")
					}
				}
			}
			stripEmptyListField(policyData, "ism_template")

			val := jsonToValue(policyData)
			resources = append(resources, provider.Resource{
				ID:   provider.ResourceID{Type: "opensearch_ism_policy", Name: entry.ID},
				Body: val.Map,
			})
		}

		from += len(resp.Policies)
		if from >= resp.TotalPolicies || len(resp.Policies) == 0 {
			break
		}
	}

	return resources, nil
}

// ismListResponse is the envelope returned by GET /_plugins/_ism/policies.
type ismListResponse struct {
	Policies []struct {
		ID     string         `json:"_id"`
		Policy map[string]any `json:"policy"`
	} `json:"policies"`
	TotalPolicies int `json:"total_policies"`
}

// Normalize strips server-injected metadata and sorts ism_template entries.
// State order is preserved — sequence matters for the ISM state machine.
func (h *ismPolicyHandler) Normalize(_ context.Context, r provider.Resource) (provider.Resource, error) {
	body := r.Body.Clone()

	// Strip server-injected metadata (belt-and-suspenders for Discover).
	body.Delete("policy_id")
	body.Delete("last_updated_time")
	body.Delete("schema_version")

	// Strip error_notification if null.
	if v, ok := body.Get("error_notification"); ok && v.Kind == provider.KindNull {
		body.Delete("error_notification")
	}

	// Strip ism_template if null — mirrors Discover.
	if v, ok := body.Get("ism_template"); ok && v.Kind == provider.KindNull {
		body.Delete("ism_template")
	}

	// Strip default retry from each action body in each state.
	if states, ok := body.Get("states"); ok && states.Kind == provider.KindList {
		for _, state := range states.List {
			if state.Kind != provider.KindMap || state.Map == nil {
				continue
			}
			actions, ok := state.Map.Get("actions")
			if !ok || actions.Kind != provider.KindList {
				continue
			}
			for _, action := range actions.List {
				if action.Kind != provider.KindMap || action.Map == nil {
					continue
				}
				if retry, ok := action.Map.Get("retry"); ok && isDefaultRetryValue(retry) {
					action.Map.Delete("retry")
				}
			}
		}
	}

	// Process ism_template entries: strip last_updated_time and sort.
	if v, ok := body.Get("ism_template"); ok && v.Kind == provider.KindList {
		for i, entry := range v.List {
			if entry.Kind == provider.KindMap {
				m := entry.Map.Clone()
				m.Delete("last_updated_time")
				v.List[i] = provider.MapVal(m)
			}
		}

		// Sort ism_template entries for deterministic diffs.
		if len(v.List) > 1 {
			slices.SortFunc(v.List, func(a, b provider.Value) int {
				aj, _ := json.Marshal(valueToJSON(a))
				bj, _ := json.Marshal(valueToJSON(b))
				return bytes.Compare(aj, bj)
			})
		}

		body.Set("ism_template", v)
	}
	stripEmptyValueList(body, "ism_template")

	return provider.Resource{ID: r.ID, Body: body, SourceRange: r.SourceRange}, nil
}

// Validate checks structural correctness of an ISM policy resource.
func (h *ismPolicyHandler) Validate(_ context.Context, r provider.Resource) error {
	prefix := fmt.Sprintf("opensearch_ism_policy.%s", r.ID.Name)

	// states — required, list of maps, each with a name.
	statesVal, ok := r.Body.Get("states")
	if !ok {
		return fmt.Errorf("%s: \"states\" is required — an ISM policy must define at least one state", prefix)
	}
	if statesVal.Kind != provider.KindList {
		return fmt.Errorf("%s: states must be a list, got %s", prefix, statesVal.Kind)
	}
	if len(statesVal.List) == 0 {
		return fmt.Errorf("%s: \"states\" must contain at least one state", prefix)
	}
	for i, state := range statesVal.List {
		if state.Kind != provider.KindMap {
			return fmt.Errorf("%s: states[%d] must be a map, got %s", prefix, i, state.Kind)
		}
		nameVal, ok := state.Map.Get("name")
		if !ok {
			return fmt.Errorf("%s: states[%d] must have a \"name\" field", prefix, i)
		}
		if nameVal.Kind != provider.KindString {
			return fmt.Errorf("%s: states[%d].name must be a string, got %s", prefix, i, nameVal.Kind)
		}
	}

	// default_state — optional string.
	if v, ok := r.Body.Get("default_state"); ok && v.Kind != provider.KindString {
		return fmt.Errorf("%s: default_state must be a string, got %s", prefix, v.Kind)
	}

	// description — optional string.
	if v, ok := r.Body.Get("description"); ok && v.Kind != provider.KindString {
		return fmt.Errorf("%s: description must be a string, got %s", prefix, v.Kind)
	}

	// ism_template — optional list of maps.
	if v, ok := r.Body.Get("ism_template"); ok {
		if v.Kind != provider.KindList {
			return fmt.Errorf("%s: ism_template must be a list, got %s", prefix, v.Kind)
		}
		for i, tmpl := range v.List {
			if tmpl.Kind != provider.KindMap {
				return fmt.Errorf("%s: ism_template[%d] must be a map, got %s", prefix, i, tmpl.Kind)
			}
		}
	}

	// error_notification — optional map (or null).
	if v, ok := r.Body.Get("error_notification"); ok && v.Kind != provider.KindNull && v.Kind != provider.KindMap {
		return fmt.Errorf("%s: error_notification must be a map, got %s", prefix, v.Kind)
	}

	return nil
}

// Apply creates, updates, or deletes an ISM policy in OpenSearch.
func (h *ismPolicyHandler) Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error {
	switch op {
	case provider.OpCreate:
		return h.putPolicy(ctx, client, op, r, "")

	case provider.OpUpdate:
		// The ISM API requires seq_no and primary_term for updates.
		// Fetch the current values before writing.
		seqParams, err := h.fetchSeqParams(ctx, client, r.ID.Name)
		if err != nil {
			return fmt.Errorf("opensearch_ism_policy.%s: %s failed: %s", r.ID.Name, op, err)
		}
		return h.putPolicy(ctx, client, op, r, seqParams)

	case provider.OpDelete:
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			"/_plugins/_ism/policies/"+r.ID.Name, nil)
		if err != nil {
			return fmt.Errorf("opensearch_ism_policy.%s: %s failed: %s", r.ID.Name, op, err)
		}

		body, status, err := client.do(req)
		if err != nil {
			return fmt.Errorf("opensearch_ism_policy.%s: %s failed: %s", r.ID.Name, op, err)
		}
		if status == http.StatusNotFound {
			return nil // already gone
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("opensearch_ism_policy.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
		}
		return nil

	default:
		return fmt.Errorf("opensearch_ism_policy.%s: unsupported operation %s", r.ID.Name, op)
	}
}

// putPolicy sends a PUT to the ISM API with the policy body wrapped in an envelope.
// queryParams is appended to the URL (e.g. "?if_seq_no=1&if_primary_term=1" for updates).
func (h *ismPolicyHandler) putPolicy(ctx context.Context, client *Client, op provider.Operation, r provider.Resource, queryParams string) error {
	policyBody := valueToJSON(provider.MapVal(r.Body))
	envelope := map[string]any{"policy": policyBody}
	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("opensearch_ism_policy.%s: %s failed: %s", r.ID.Name, op, err)
	}

	url := "/_plugins/_ism/policies/" + r.ID.Name + queryParams
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("opensearch_ism_policy.%s: %s failed: %s", r.ID.Name, op, err)
	}
	req.Header.Set("Content-Type", "application/json")

	body, status, err := client.do(req)
	if err != nil {
		return fmt.Errorf("opensearch_ism_policy.%s: %s failed: %s", r.ID.Name, op, err)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("opensearch_ism_policy.%s: %s failed (%d): %s", r.ID.Name, op, status, body)
	}
	return nil
}

// fetchSeqParams retrieves the current _seq_no and _primary_term for a policy
// and returns them as a query string for the PUT update request.
func (h *ismPolicyHandler) fetchSeqParams(ctx context.Context, client *Client, name string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/_plugins/_ism/policies/"+name, nil)
	if err != nil {
		return "", err
	}

	body, status, err := client.do(req)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("failed to fetch policy for update (%d): %s", status, body)
	}

	var resp struct {
		SeqNo       int64 `json:"_seq_no"`
		PrimaryTerm int64 `json:"_primary_term"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse policy metadata: %s", err)
	}

	return fmt.Sprintf("?if_seq_no=%d&if_primary_term=%d", resp.SeqNo, resp.PrimaryTerm), nil
}
