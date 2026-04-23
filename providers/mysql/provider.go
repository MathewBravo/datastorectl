package mysql

import (
	"context"
	"fmt"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

func init() {
	provider.Register("mysql", func() provider.Provider {
		return &Provider{
			handlers: map[string]resourceHandler{
				"mysql_user":     &stubHandler{typeName: "mysql_user"},
				"mysql_grant":    &stubHandler{typeName: "mysql_grant"},
				"mysql_role":     &stubHandler{typeName: "mysql_role"},
				"mysql_database": &stubHandler{typeName: "mysql_database"},
			},
		}
	})
}

// Provider implements provider.Provider for MySQL clusters. The Phase 18
// scaffold wires registration, handler dispatch, and placeholder
// diagnostics. Real configuration, discovery, and application land in
// subsequent phases.
type Provider struct {
	handlers map[string]resourceHandler
}

// Configure is not yet implemented. The scaffold returns a deterministic
// diagnostic so integration code can detect the placeholder state
// without crashing.
func (p *Provider) Configure(_ context.Context, _ *provider.OrderedMap) dcl.Diagnostics {
	return dcl.Diagnostics{{
		Severity: dcl.SeverityError,
		Message:  "mysql provider Configure is not implemented yet (Phase 18 scaffold)",
	}}
}

// Discover iterates registered handlers and collects any discovered
// resources. In the scaffold every handler errors, so this returns an
// aggregated diagnostics list.
func (p *Provider) Discover(ctx context.Context) ([]provider.Resource, dcl.Diagnostics) {
	var all []provider.Resource
	var diags dcl.Diagnostics
	for _, h := range p.handlers {
		resources, err := h.Discover(ctx)
		if err != nil {
			diags = append(diags, dcl.Diagnostic{Severity: dcl.SeverityError, Message: err.Error()})
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
			Message:  fmt.Sprintf("resource type %q is not supported by the mysql provider", r.ID.Type),
		}}
	}
	out, err := h.Normalize(ctx, r)
	if err != nil {
		return r, dcl.Diagnostics{{Severity: dcl.SeverityError, Message: err.Error()}}
	}
	return out, nil
}

// Validate delegates to the handler registered for the resource type.
func (p *Provider) Validate(ctx context.Context, r provider.Resource) dcl.Diagnostics {
	h, ok := p.handlers[r.ID.Type]
	if !ok {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf("resource type %q is not supported by the mysql provider", r.ID.Type),
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
			Message:  fmt.Sprintf("resource type %q is not supported by the mysql provider", r.ID.Type),
		}}
	}
	if err := h.Apply(ctx, op, r); err != nil {
		return dcl.Diagnostics{{Severity: dcl.SeverityError, Message: err.Error()}}
	}
	return nil
}

// Schemas collects schema declarations from handlers that implement
// schemaProvider. Scaffold handlers do not yet declare schemas, so this
// returns an empty map until real handlers land.
func (p *Provider) Schemas() map[string]provider.Schema {
	schemas := make(map[string]provider.Schema)
	for typ, h := range p.handlers {
		if sp, ok := h.(schemaProvider); ok {
			schemas[typ] = sp.Schema()
		}
	}
	return schemas
}

// TypeOrderings declares the default resource type ordering for the
// mysql provider. Per ADR 0008: users before grants and roles, roles
// before grants (so role-to-role edges see the role created). Databases
// have no hard ordering dependency with the others.
func (p *Provider) TypeOrderings() []provider.TypeOrdering {
	return []provider.TypeOrdering{
		{Before: "mysql_user", After: "mysql_grant"},
		{Before: "mysql_user", After: "mysql_role"},
		{Before: "mysql_role", After: "mysql_grant"},
	}
}
