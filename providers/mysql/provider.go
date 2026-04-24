package mysql

import (
	"context"
	"fmt"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// validTLSModes lists the accepted values for the `tls` field.
var validTLSModes = map[string]bool{
	"required":    true,
	"skip-verify": true,
	"disabled":    true,
}

// supportedVersions lists the MySQL major.minor targets this release
// accepts. Additive extension: new versions land here as minor changes
// alongside any new version-gated schema fields.
var supportedVersions = map[string]bool{
	"8.0": true,
	"8.4": true,
}

// deferredVersions maps known-but-not-yet-supported MySQL versions to
// tracking info so the rejection diagnostic points users at the right
// place to follow along.
var deferredVersions = map[string]string{
	"5.7": `MySQL 5.7 support is tracked as a post-v0.1.0 addition`,
}

func init() {
	provider.Register("mysql", func() provider.Provider {
		return &Provider{
			handlers: map[string]resourceHandler{
				"mysql_user":     &stubHandler{typeName: "mysql_user"},
				"mysql_grant":    &stubHandler{typeName: "mysql_grant"},
				"mysql_role":     &roleHandler{},
				"mysql_database": &databaseHandler{},
			},
		}
	})
}

// Provider implements provider.Provider for MySQL clusters.
type Provider struct {
	client   *Client
	version  string // declared major.minor target ("8.0" | "8.4")
	handlers map[string]resourceHandler
}

// Version returns the declared target MySQL version the provider was
// configured against. Empty before Configure runs.
func (p *Provider) Version() string {
	return p.version
}

// Configure validates the provider block and opens the underlying
// *sql.DB connection. Supported auth modes are "password" (Phase 18)
// and "rds_iam" (Phase 24). The connection pool is sized at one.
func (p *Provider) Configure(ctx context.Context, config *provider.OrderedMap) dcl.Diagnostics {
	if config == nil {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  `mysql provider requires configuration — set at minimum "endpoint", "auth", and the credentials for your chosen auth method`,
		}}
	}

	endpoint, diags := requireStringField(config, "endpoint",
		`"endpoint" is required and must be a non-empty string (e.g. "mysql.example.com:3306")`,
		`add endpoint = "your-mysql-host:3306" to the mysql provider block`)
	if diags.HasErrors() {
		return diags
	}

	auth, diags := requireStringField(config, "auth",
		`"auth" is required — set it to "password" for username/password or "rds_iam" for AWS RDS IAM authentication`,
		`add auth = "password" or auth = "rds_iam" to the mysql provider block`)
	if diags.HasErrors() {
		return diags
	}

	tlsMode, diags := resolveTLSField(config)
	if diags.HasErrors() {
		return diags
	}

	version, diags := resolveVersionField(config)
	if diags.HasErrors() {
		return diags
	}
	p.version = version

	switch auth {
	case "password":
		return p.configurePasswordAuth(ctx, endpoint, tlsMode, config)
	case "rds_iam":
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    `auth = "rds_iam" is not implemented yet`,
			Suggestion: `use auth = "password" until Phase 24 lands RDS IAM support`,
		}}
	default:
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf(`auth must be "password" or "rds_iam", got %q`, auth),
		}}
	}
}

// configurePasswordAuth validates the password-auth required fields,
// constructs the client, and runs a sanity ping to confirm the
// connection is live.
func (p *Provider) configurePasswordAuth(ctx context.Context, endpoint, tlsMode string, config *provider.OrderedMap) dcl.Diagnostics {
	username, diags := requireStringField(config, "username",
		`"username" is required when auth is "password"`,
		`add username = "datastorectl" to the mysql provider block`)
	if diags.HasErrors() {
		return diags
	}

	password, diags := requireStringField(config, "password",
		`"password" is required when auth is "password"`,
		`add password = secret("env", "MYSQL_PASSWORD") to the mysql provider block`)
	if diags.HasErrors() {
		return diags
	}

	cfg := ClientConfig{
		Endpoint: endpoint,
		Username: username,
		Password: password,
		TLS:      tlsMode,
		TLSCA:    optionalString(config, "tls_ca"),
		TLSCert:  optionalString(config, "tls_cert"),
		TLSKey:   optionalString(config, "tls_key"),
	}

	client, err := NewPasswordClient(cfg)
	if err != nil {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf("failed to create mysql client: %s", err),
		}}
	}

	if err := client.DB().PingContext(ctx); err != nil {
		_ = client.Close()
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    fmt.Sprintf("mysql connection check failed: %s", err),
			Suggestion: "verify the endpoint is reachable, credentials are correct, and TLS settings match the server",
		}}
	}

	p.client = client

	if diags := p.verifyServerVersion(ctx); diags.HasErrors() {
		_ = client.Close()
		p.client = nil
		return diags
	}
	return nil
}

// verifyServerVersion queries the server's reported VERSION() string
// and compares its major.minor against the declared target. Patch-level
// drift and vendor suffixes (e.g. "8.4.5-log", "8.0.35-aurora.3") are
// tolerated; only major.minor mismatch fails.
func (p *Provider) verifyServerVersion(ctx context.Context) dcl.Diagnostics {
	var serverVersion string
	if err := p.client.DB().QueryRowContext(ctx, "SELECT VERSION()").Scan(&serverVersion); err != nil {
		return dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    fmt.Sprintf("failed to query server version: %s", err),
			Suggestion: "verify the connected user can execute SELECT VERSION() (it's available to any authenticated user)",
		}}
	}
	cmp, err := provider.CompareVersions(serverVersion, p.version)
	if err != nil {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  fmt.Sprintf("could not parse server version %q: %s", serverVersion, err),
		}}
	}
	if cmp != 0 {
		return dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message: fmt.Sprintf(
				`server reports %s but context targets %q`,
				serverVersion, p.version,
			),
			Suggestion: fmt.Sprintf(
				`either update the provider block to version = "%s", or upgrade/downgrade the cluster before applying this config`,
				majorMinor(serverVersion),
			),
		}}
	}
	return nil
}

// majorMinor returns the "M.N" prefix of a version string, used only
// for operator-facing suggestion messages. Falls back to the input
// when parsing fails so messages never garble.
func majorMinor(v string) string {
	out := ""
	dots := 0
	for _, r := range v {
		if r == '.' {
			dots++
			if dots == 2 {
				break
			}
		}
		if r >= '0' && r <= '9' || r == '.' {
			out += string(r)
			continue
		}
		break
	}
	if dots < 1 {
		return v
	}
	return out
}

// requireStringField fetches a required string field, producing a
// standard diagnostic when missing or empty.
func requireStringField(config *provider.OrderedMap, name, message, suggestion string) (string, dcl.Diagnostics) {
	v, ok := config.Get(name)
	if !ok || v.Kind != provider.KindString || v.Str == "" {
		return "", dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    message,
			Suggestion: suggestion,
		}}
	}
	return v.Str, nil
}

// optionalString returns the string value of a field if it exists and
// is a non-empty string, otherwise "".
func optionalString(config *provider.OrderedMap, name string) string {
	v, ok := config.Get(name)
	if !ok || v.Kind != provider.KindString {
		return ""
	}
	return v.Str
}

// resolveVersionField parses the required `version` enum. Per ADR
// 0009, the user must commit to a supported MySQL major.minor target.
// Known-deferred versions produce a specific diagnostic pointing at
// the tracking issue.
func resolveVersionField(config *provider.OrderedMap) (string, dcl.Diagnostics) {
	v, ok := config.Get("version")
	if !ok {
		return "", dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    `"version" is required — declare the MySQL major.minor target so datastorectl can gate version-specific attributes at validate time`,
			Suggestion: `add version = "8.4" (LTS) or version = "8.0" to the mysql provider block`,
		}}
	}
	if v.Kind != provider.KindString {
		return "", dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  `"version" must be a string (e.g. "8.4")`,
		}}
	}
	version := v.Str
	if reason, deferred := deferredVersions[version]; deferred {
		return "", dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    fmt.Sprintf(`version %q is not supported in this release: %s`, version, reason),
			Suggestion: `use version = "8.0" or version = "8.4" for now`,
		}}
	}
	if !supportedVersions[version] {
		return "", dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    fmt.Sprintf(`version must be "8.0" or "8.4", got %q`, version),
			Suggestion: `use version = "8.4" for the current MySQL LTS line`,
		}}
	}
	return version, nil
}

// resolveTLSField parses the optional `tls` enum, defaulting to
// "required" when absent.
func resolveTLSField(config *provider.OrderedMap) (string, dcl.Diagnostics) {
	v, ok := config.Get("tls")
	if !ok {
		return "required", nil
	}
	if v.Kind != provider.KindString {
		return "", dcl.Diagnostics{{
			Severity: dcl.SeverityError,
			Message:  `"tls" must be a string`,
		}}
	}
	mode := v.Str
	if mode == "" {
		return "required", nil
	}
	if !validTLSModes[mode] {
		return "", dcl.Diagnostics{{
			Severity:   dcl.SeverityError,
			Message:    fmt.Sprintf(`"tls" must be "required", "skip-verify", or "disabled", got %q`, mode),
			Suggestion: `use tls = "required" to verify the server certificate (default), "skip-verify" to connect without verification, or "disabled" to connect in plaintext`,
		}}
	}
	return mode, nil
}

// Discover iterates registered handlers and collects any discovered
// resources. Each handler receives the shared client configured at
// Configure time.
func (p *Provider) Discover(ctx context.Context) ([]provider.Resource, dcl.Diagnostics) {
	var all []provider.Resource
	var diags dcl.Diagnostics
	for _, h := range p.handlers {
		resources, err := h.Discover(ctx, p.client)
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
	if err := h.Apply(ctx, p.client, op, r); err != nil {
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
