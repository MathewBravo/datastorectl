package opensearch

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"testing"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// resourceAPIPaths maps resource types to their OpenSearch REST API path prefixes.
var resourceAPIPaths = map[string]string{
	"opensearch_role":                      "/_plugins/_security/api/roles/",
	"opensearch_role_mapping":              "/_plugins/_security/api/rolesmapping/",
	"opensearch_internal_user":             "/_plugins/_security/api/internalusers/",
	"opensearch_component_template":        "/_component_template/",
	"opensearch_composable_index_template": "/_index_template/",
}

// restPath returns the full REST path for a resource type and name.
// Panics on unknown resource types — test setup bugs should be loud.
func restPath(resourceType, name string) string {
	prefix, ok := resourceAPIPaths[resourceType]
	if !ok {
		panic(fmt.Sprintf("unknown resource type in test helper: %q", resourceType))
	}
	return prefix + name
}

// newTestClient creates a *Client connected to the integration-test cluster.
// Skips the test when no cluster is available.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	skipIfNoCluster(t)

	apiClient, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: []string{testEndpoint},
			Username:  testUsername,
			Password:  testPassword,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create test OpenSearch client: %v", err)
	}

	return &Client{api: apiClient}
}

// cleanupResource registers a t.Cleanup that deletes the named resource.
// A 404 is silently ignored (resource may not have been created).
func cleanupResource(t *testing.T, client *Client, resourceType, name string) {
	t.Helper()
	t.Cleanup(func() {
		path := restPath(resourceType, name)
		req, err := http.NewRequest(http.MethodDelete, path, nil)
		if err != nil {
			t.Logf("cleanup: failed to build DELETE request for %s/%s: %v", resourceType, name, err)
			return
		}

		resp, err := client.api.Client.Perform(req)
		if err != nil {
			t.Logf("cleanup: DELETE %s failed: %v", path, err)
			return
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound && resp.StatusCode >= 400 {
			t.Logf("cleanup: DELETE %s returned %d", path, resp.StatusCode)
		}
	})
}

// requireResourceExists asserts the resource exists in the cluster.
func requireResourceExists(t *testing.T, client *Client, resourceType, name string) {
	t.Helper()

	path := restPath(resourceType, name)
	req, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		t.Fatalf("requireResourceExists: failed to build GET request: %v", err)
	}

	resp, err := client.api.Client.Perform(req)
	if err != nil {
		t.Fatalf("requireResourceExists: GET %s failed: %v", path, err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("requireResourceExists: %s/%s does not exist (404)", resourceType, name)
	}
	if resp.StatusCode >= 400 {
		t.Fatalf("requireResourceExists: GET %s returned %d", path, resp.StatusCode)
	}
}

// requireResourceNotExists asserts the resource does not exist in the cluster.
func requireResourceNotExists(t *testing.T, client *Client, resourceType, name string) {
	t.Helper()

	path := restPath(resourceType, name)
	req, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		t.Fatalf("requireResourceNotExists: failed to build GET request: %v", err)
	}

	resp, err := client.api.Client.Perform(req)
	if err != nil {
		t.Fatalf("requireResourceNotExists: GET %s failed: %v", path, err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return // expected
	}
	if resp.StatusCode < 400 {
		t.Fatalf("requireResourceNotExists: %s/%s unexpectedly exists (status %d)", resourceType, name, resp.StatusCode)
	}
	t.Fatalf("requireResourceNotExists: GET %s returned unexpected status %d", path, resp.StatusCode)
}
