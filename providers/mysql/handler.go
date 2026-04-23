package mysql

import (
	"context"
	"errors"

	"github.com/MathewBravo/datastorectl/provider"
)

// resourceHandler is the internal interface each resource type implements.
// The top-level Provider dispatches to the matching handler by resource type.
type resourceHandler interface {
	Discover(ctx context.Context) ([]provider.Resource, error)
	Normalize(ctx context.Context, r provider.Resource) (provider.Resource, error)
	Validate(ctx context.Context, r provider.Resource) error
	Apply(ctx context.Context, op provider.Operation, r provider.Resource) error
}

// schemaProvider is an optional interface a handler may implement to declare
// the expected structure of its nested blocks. The provider collects these
// during Schemas() and passes them to the DCL converter.
type schemaProvider interface {
	Schema() provider.Schema
}

// errNotImplemented is returned by scaffold handlers until the real
// implementation lands in later phases.
var errNotImplemented = errors.New("mysql provider handler is not implemented yet")

// stubHandler is the placeholder used for every resource type in the
// Phase 18 scaffold. Every method returns errNotImplemented so callers
// get a deterministic signal that the handler is registered but empty.
type stubHandler struct {
	typeName string
}

func (h *stubHandler) Discover(ctx context.Context) ([]provider.Resource, error) {
	return nil, errNotImplemented
}

func (h *stubHandler) Normalize(ctx context.Context, r provider.Resource) (provider.Resource, error) {
	return r, errNotImplemented
}

func (h *stubHandler) Validate(ctx context.Context, r provider.Resource) error {
	return errNotImplemented
}

func (h *stubHandler) Apply(ctx context.Context, op provider.Operation, r provider.Resource) error {
	return errNotImplemented
}
