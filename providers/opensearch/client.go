package opensearch

import (
	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// Client is a thin wrapper around the opensearch-go client.
type Client struct {
	api *opensearchapi.Client
}

// NewClient creates an OpenSearch client configured with basic auth.
func NewClient(endpoint, username, password string) (*Client, error) {
	api, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: []string{endpoint},
			Username:  username,
			Password:  password,
		},
	})
	if err != nil {
		return nil, err
	}
	return &Client{api: api}, nil
}
