package opensearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// callerIdentity captures the authenticated principal on the OpenSearch
// cluster, as reported by /_plugins/_security/api/account. Used to
// classify role_mapping and internal_user deletes that would lock out
// the caller.
type callerIdentity struct {
	UserName     string
	BackendRoles []string
}

// fetchCallerIdentity queries /_plugins/_security/api/account and returns
// the authenticated user's name and backend roles. Called once during
// provider Configure.
func fetchCallerIdentity(ctx context.Context, client *Client) (callerIdentity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_plugins/_security/api/account", nil)
	if err != nil {
		return callerIdentity{}, fmt.Errorf("opensearch: caller identity: %s", err)
	}

	body, status, err := client.do(req)
	if err != nil {
		return callerIdentity{}, fmt.Errorf("opensearch: caller identity: %s", err)
	}
	if status < 200 || status >= 300 {
		return callerIdentity{}, fmt.Errorf("opensearch: caller identity failed (%d): %s", status, body)
	}

	var raw struct {
		UserName     string   `json:"user_name"`
		BackendRoles []string `json:"backend_roles"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return callerIdentity{}, fmt.Errorf("opensearch: caller identity: decode: %s", err)
	}

	return callerIdentity{
		UserName:     raw.UserName,
		BackendRoles: raw.BackendRoles,
	}, nil
}
