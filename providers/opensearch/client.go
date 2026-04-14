package opensearch

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"github.com/opensearch-project/opensearch-go/v4/signer/awsv2"
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

// NewSigV4Client creates an OpenSearch client that signs requests with AWS SigV4.
// It uses the default AWS credential chain (env vars → shared credentials → IAM role → SSO).
func NewSigV4Client(ctx context.Context, endpoint, region string) (*Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf(
			"failed to load AWS credentials — ensure AWS credentials are available "+
				"(via environment variables, ~/.aws/credentials, IAM role, or SSO): %w", err,
		)
	}

	signer, err := awsv2.NewSigner(awsCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS SigV4 request signer: %w", err)
	}

	api, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: []string{endpoint},
			Signer:    signer,
		},
	})
	if err != nil {
		return nil, err
	}
	return &Client{api: api}, nil
}
