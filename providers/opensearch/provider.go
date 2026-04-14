package opensearch

import (
	"context"
	"fmt"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

func init() {
	provider.Register("opensearch", func() provider.Provider {
		return &Provider{
			handlers: map[string]resourceHandler{
				// Resource handlers will be registered here as they're implemented.
			},
		}
	})
}

// Provider implements provider.Provider for OpenSearch clusters.
type Provider struct {
	client   *Client
	handlers map[string]resourceHandler
}

// Configure validates the provider configuration and creates the HTTP client.
func (p *Provider) Configure(_ context.Context, config *provider.OrderedMap) dcl.Diagnostics {
	if config == nil {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  `opensearch provider requires configuration — set at minimum "endpoint", "auth", "username", and "password"`,
		}}
	}

	// endpoint — required, non-empty string.
	endpointVal, ok := config.Get("endpoint")
	if !ok || endpointVal.Kind != provider.KindString || endpointVal.Str == "" {
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    `"endpoint" is required and must be a non-empty string (the OpenSearch cluster URL, e.g. "https://search.example.com:9200")`,
			Suggestion: `add endpoint = "https://your-cluster:9200" to the opensearch provider block`,
		}}
	}
	endpoint := endpointVal.Str

	// auth — required, must be "basic".
	authVal, ok := config.Get("auth")
	if !ok || authVal.Kind != provider.KindString || authVal.Str == "" {
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    `"auth" is required — set it to "basic" to use username/password authentication`,
			Suggestion: `add auth = "basic" to the opensearch provider block`,
		}}
	}
	auth := authVal.Str

	switch auth {
	case "basic":
		// OK — fall through to credential validation below.
	case "sigv4":
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  `auth is "sigv4" but sigv4 support is not yet implemented — use "basic" for now`,
		}}
	default:
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf(`auth must be "basic" (sigv4 support is planned but not yet available), got %q`, auth),
		}}
	}

	// username — required for basic auth.
	usernameVal, ok := config.Get("username")
	if !ok || usernameVal.Kind != provider.KindString || usernameVal.Str == "" {
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    `"username" is required when auth is "basic"`,
			Suggestion: `add username = "admin" to the opensearch provider block`,
		}}
	}
	username := usernameVal.Str

	// password — required for basic auth.
	passwordVal, ok := config.Get("password")
	if !ok || passwordVal.Kind != provider.KindString || passwordVal.Str == "" {
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    `"password" is required when auth is "basic"`,
			Suggestion: `add password = secret("opensearch_password") to the opensearch provider block`,
		}}
	}
	password := passwordVal.Str

	client, err := NewClient(endpoint, username, password)
	if err != nil {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf("failed to create opensearch client: %s", err),
		}}
	}
	p.client = client
	return nil
}

// Discover iterates all registered handlers and collects discovered resources.
func (p *Provider) Discover(ctx context.Context) ([]provider.Resource, dcl.Diagnostics) {
	var all []provider.Resource
	var diags dcl.Diagnostics
	for _, h := range p.handlers {
		resources, err := h.Discover(ctx, p.client)
		if err != nil {
			diags.Append(dcl.Diagnostics{{Severity: dcl.SeverityError, Message: err.Error()}})
			continue
		}
		all = append(all, resources...)
	}
	return all, diags
}

// Normalize delegates to the handler registered for the resource type.
func (p *Provider) Normalize(ctx context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
	h, ok := p.handlers[r.ID.Type]
	if !ok {
		return r, dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf("resource type %q is not supported by the opensearch provider", r.ID.Type),
		}}
	}
	result, err := h.Normalize(ctx, r)
	if err != nil {
		return r, dcl.Diagnostics{{Severity: dcl.SeverityError, Message: err.Error()}}
	}
	return result, nil
}

// Validate delegates to the handler registered for the resource type.
func (p *Provider) Validate(ctx context.Context, r provider.Resource) dcl.Diagnostics {
	h, ok := p.handlers[r.ID.Type]
	if !ok {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf("resource type %q is not supported by the opensearch provider", r.ID.Type),
		}}
	}
	if err := h.Validate(ctx, r); err != nil {
		return dcl.Diagnostics{{Severity: dcl.SeverityError, Message: err.Error()}}
	}
	return nil
}

// Apply delegates to the handler registered for the resource type.
func (p *Provider) Apply(ctx context.Context, op provider.Operation, r provider.Resource) dcl.Diagnostics {
	h, ok := p.handlers[r.ID.Type]
	if !ok {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf("resource type %q is not supported by the opensearch provider", r.ID.Type),
		}}
	}
	if err := h.Apply(ctx, p.client, op, r); err != nil {
		return dcl.Diagnostics{{Severity: dcl.SeverityError, Message: err.Error()}}
	}
	return nil
}

// TypeOrderings declares the default resource type ordering for the opensearch provider.
func (p *Provider) TypeOrderings() []provider.TypeOrdering {
	return []provider.TypeOrdering{
		{Before: "opensearch_role", After: "opensearch_role_mapping"},
		{Before: "opensearch_internal_user", After: "opensearch_role_mapping"},
		{Before: "opensearch_component_template", After: "opensearch_composable_index_template"},
	}
}
