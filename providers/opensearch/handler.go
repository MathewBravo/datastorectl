package opensearch

import (
	"context"

	"github.com/MathewBravo/datastorectl/provider"
)

// resourceHandler is the internal interface each resource type implements.
// The top-level Provider dispatches to the matching handler by resource type.
type resourceHandler interface {
	Discover(ctx context.Context, client *Client) ([]provider.Resource, error)
	Normalize(ctx context.Context, r provider.Resource) (provider.Resource, error)
	Validate(ctx context.Context, r provider.Resource) error
	Apply(ctx context.Context, client *Client, op provider.Operation, r provider.Resource) error
}

// schemaProvider is an optional interface a handler may implement to declare
// the expected structure of its nested blocks. The provider collects these
// during Schemas() and passes them to the DCL converter.
type schemaProvider interface {
	Schema() provider.Schema
}
