package opensearch

import (
	"crypto/tls"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	defaultTestEndpoint = "https://localhost:9200"
	testUsername         = "admin"
	testPassword         = "myStrongPassword123!"
)

// testEndpoint holds the resolved OpenSearch endpoint.
// Empty means no cluster is available — integration tests will be skipped.
var testEndpoint string

func TestMain(m *testing.M) {
	endpoint := os.Getenv("DATASTORECTL_TEST_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultTestEndpoint
	}

	if clusterReachable(endpoint) {
		testEndpoint = endpoint
	} else {
		// Log once so CI output shows why integration tests were skipped.
		// Use Stderr so it appears even without -v.
		os.Stderr.WriteString("opensearch: cluster not reachable at " + endpoint +
			" — skipping integration tests (set DATASTORECTL_TEST_ENDPOINT or run docker compose up)\n")
	}

	os.Exit(m.Run())
}

// clusterReachable sends a HEAD request to the endpoint and returns true if
// the cluster responds. Uses raw net/http so a bug in opensearch-go cannot
// break the skip logic.
func clusterReachable(endpoint string) bool {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest(http.MethodHead, endpoint, nil)
	if err != nil {
		return false
	}
	req.SetBasicAuth(testUsername, testPassword)

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// skipIfNoCluster skips t when no OpenSearch cluster is available.
func skipIfNoCluster(t *testing.T) {
	t.Helper()
	if testEndpoint == "" {
		t.Skip("no OpenSearch cluster available; set DATASTORECTL_TEST_ENDPOINT or run docker compose up")
	}
}
