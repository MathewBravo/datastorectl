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
				"opensearch_role":              &roleHandler{},
				"opensearch_internal_user":     &internalUserHandler{},
				"opensearch_role_mapping":      &roleMappingHandler{},
				"opensearch_ism_policy":        &ismPolicyHandler{},
				"opensearch_cluster_settings":  &clusterSettingsHandler{},
				"opensearch_component_template":        &componentTemplateHandler{},
				"opensearch_composable_index_template": &composableIndexTemplateHandler{},
				"opensearch_ingest_pipeline":           &ingestPipelineHandler{},
				"opensearch_snapshot_repository":       &snapshotRepositoryHandler{},
			},
		}
	})
}

// Provider implements provider.Provider for OpenSearch clusters.
type Provider struct {
	client   *Client
	caller   callerIdentity
	handlers map[string]resourceHandler
}

// Configure validates the provider configuration and creates the HTTP client.
func (p *Provider) Configure(ctx context.Context, config *provider.OrderedMap) dcl.Diagnostics {
	if config == nil {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  `opensearch provider requires configuration — set at minimum "endpoint", "auth", and the credentials for your chosen auth method`,
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

	// auth — required, must be "basic" or "sigv4".
	authVal, ok := config.Get("auth")
	if !ok || authVal.Kind != provider.KindString || authVal.Str == "" {
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    `"auth" is required — set it to "basic" for username/password or "sigv4" for AWS IAM authentication`,
			Suggestion: `add auth = "basic" or auth = "sigv4" to the opensearch provider block`,
		}}
	}
	auth := authVal.Str

	switch auth {
	case "basic":
		return p.configureBasicAuth(ctx, endpoint, config)
	case "sigv4":
		return p.configureSigV4(ctx, endpoint, config)
	default:
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf(`auth must be "basic" or "sigv4", got %q`, auth),
		}}
	}
}

// tlsSkipVerify returns true if tls_skip_verify is set to true in the config.
func tlsSkipVerify(config *provider.OrderedMap) bool {
	v, ok := config.Get("tls_skip_verify")
	if !ok {
		return false
	}
	return v.Kind == provider.KindBool && v.Bool
}

// configureBasicAuth validates username/password and creates a basic-auth client.
func (p *Provider) configureBasicAuth(ctx context.Context, endpoint string, config *provider.OrderedMap) dcl.Diagnostics {
	usernameVal, ok := config.Get("username")
	if !ok || usernameVal.Kind != provider.KindString || usernameVal.Str == "" {
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    `"username" is required when auth is "basic"`,
			Suggestion: `add username = "admin" to the opensearch provider block`,
		}}
	}

	passwordVal, ok := config.Get("password")
	if !ok || passwordVal.Kind != provider.KindString || passwordVal.Str == "" {
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    `"password" is required when auth is "basic"`,
			Suggestion: `add password = secret("opensearch_password") to the opensearch provider block`,
		}}
	}

	client, err := NewClient(endpoint, usernameVal.Str, passwordVal.Str, tlsSkipVerify(config))
	if err != nil {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf("failed to create opensearch client: %s", err),
		}}
	}
	p.client = client
	return p.fetchAndCacheCallerIdentity(ctx)
}

// configureSigV4 validates the region field and creates a SigV4-signing client.
func (p *Provider) configureSigV4(ctx context.Context, endpoint string, config *provider.OrderedMap) dcl.Diagnostics {
	regionVal, ok := config.Get("region")
	if !ok || regionVal.Kind != provider.KindString || regionVal.Str == "" {
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    `"region" is required when auth is "sigv4" — this is the AWS region where your OpenSearch domain is deployed (e.g. "us-east-1")`,
			Suggestion: `add region = "us-east-1" to the opensearch provider block`,
		}}
	}

	client, err := NewSigV4Client(ctx, endpoint, regionVal.Str, tlsSkipVerify(config))
	if err != nil {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf("failed to create opensearch client with SigV4 auth: %s", err),
		}}
	}
	p.client = client
	return p.fetchAndCacheCallerIdentity(ctx)
}

// fetchAndCacheCallerIdentity hits /_plugins/_security/api/account and
// caches the caller's user_name and backend_roles for self-lockout
// protection. Called at the end of both auth paths.
func (p *Provider) fetchAndCacheCallerIdentity(ctx context.Context) dcl.Diagnostics {
	id, err := fetchCallerIdentity(ctx, p.client)
	if err != nil {
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    fmt.Sprintf("opensearch: unable to fetch caller identity for self-lockout protection: %s", err),
			Suggestion: "verify the credentials have access to /_plugins/_security/api/account",
		}}
	}
	p.caller = id
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

// Schemas collects schema declarations from all handlers that implement the
// schemaProvider interface and returns them keyed by resource type.
func (p *Provider) Schemas() map[string]provider.Schema {
	schemas := make(map[string]provider.Schema)
	for typ, h := range p.handlers {
		if sp, ok := h.(schemaProvider); ok {
			schemas[typ] = sp.Schema()
		}
	}
	return schemas
}

// GuardDeletes implements provider.DeleteGuarder. It flags any delete
// that would lock out the authenticated caller — either a role_mapping
// whose users/backend_roles cover the caller, or the caller's own
// internal_user account.
func (p *Provider) GuardDeletes(_ context.Context, deletes []provider.Resource) ([]provider.DeleteGuard, dcl.Diagnostics) {
	var guards []provider.DeleteGuard
	for _, r := range deletes {
		switch r.ID.Type {
		case "opensearch_role_mapping":
			if classifyRoleMappingLockout(r, p.caller) {
				guards = append(guards, provider.DeleteGuard{
					Resource: r.ID,
					Reason:   fmt.Sprintf("would revoke caller %q access (mapping grants privileges the caller currently uses)", p.caller.UserName),
				})
			}
		case "opensearch_internal_user":
			if classifyInternalUserLockout(r, p.caller) {
				guards = append(guards, provider.DeleteGuard{
					Resource: r.ID,
					Reason:   fmt.Sprintf("would delete caller %q's own user account", p.caller.UserName),
				})
			}
		}
	}
	return guards, nil
}

// TypeOrderings declares the default resource type ordering for the opensearch provider.
func (p *Provider) TypeOrderings() []provider.TypeOrdering {
	return []provider.TypeOrdering{
		{Before: "opensearch_role", After: "opensearch_role_mapping"},
		{Before: "opensearch_internal_user", After: "opensearch_role_mapping"},
		{Before: "opensearch_component_template", After: "opensearch_composable_index_template"},
	}
}
