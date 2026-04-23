package opensearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// newAccountAwareServer returns an httptest server whose
// /_plugins/_security/api/account endpoint returns a stub identity.
// Other paths return 404. Used by Configure success tests that need
// the post-client caller-identity fetch to succeed.
func newAccountAwareServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_plugins/_security/api/account" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"user_name":"test","backend_roles":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
}

// helper builds an *OrderedMap from key-value string pairs.
func configMap(kvs ...string) *provider.OrderedMap {
	m := provider.NewOrderedMap()
	for i := 0; i < len(kvs); i += 2 {
		m.Set(kvs[i], provider.StringVal(kvs[i+1]))
	}
	return m
}

// configMapWithBool builds an *OrderedMap from key-value pairs of mixed types (string or bool).
func configMapWithBool(kvs ...any) *provider.OrderedMap {
	m := provider.NewOrderedMap()
	for i := 0; i < len(kvs); i += 2 {
		key := kvs[i].(string)
		switch v := kvs[i+1].(type) {
		case string:
			m.Set(key, provider.StringVal(v))
		case bool:
			m.Set(key, provider.BoolVal(v))
		}
	}
	return m
}

// ─── Configure validation ───────────────────────────────────────────

func TestConfigure(t *testing.T) {
	tests := []struct {
		name      string
		config    *provider.OrderedMap
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "nil_config",
			config:    nil,
			wantErr:   true,
			errSubstr: "requires configuration",
		},
		{
			name:      "missing_endpoint",
			config:    configMap("auth", "basic", "username", "admin", "password", "secret"),
			wantErr:   true,
			errSubstr: "endpoint",
		},
		{
			name:      "empty_endpoint",
			config:    configMap("endpoint", "", "auth", "basic", "username", "admin", "password", "secret"),
			wantErr:   true,
			errSubstr: "endpoint",
		},
		{
			name:      "missing_auth",
			config:    configMap("endpoint", "https://localhost:9200"),
			wantErr:   true,
			errSubstr: "auth",
		},
		{
			name:      "invalid_auth",
			config:    configMap("endpoint", "https://localhost:9200", "auth", "oauth"),
			wantErr:   true,
			errSubstr: `"basic" or "sigv4"`,
		},
		{
			name:      "sigv4_missing_region",
			config:    configMap("endpoint", "https://search.us-east-1.es.amazonaws.com", "auth", "sigv4"),
			wantErr:   true,
			errSubstr: "region",
		},
		{
			name:      "basic_missing_username",
			config:    configMap("endpoint", "https://localhost:9200", "auth", "basic", "password", "secret"),
			wantErr:   true,
			errSubstr: "username",
		},
		{
			name:      "basic_missing_password",
			config:    configMap("endpoint", "https://localhost:9200", "auth", "basic", "username", "admin"),
			wantErr:   true,
			errSubstr: "password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{handlers: map[string]resourceHandler{}}
			diags := p.Configure(context.Background(), tt.config)
			if tt.wantErr {
				if !diags.HasErrors() {
					t.Fatal("expected error diagnostic, got none")
				}
				msg := diags.Error()
				if !strings.Contains(msg, tt.errSubstr) {
					t.Errorf("expected error to contain %q, got:\n%s", tt.errSubstr, msg)
				}
			} else {
				if diags.HasErrors() {
					t.Fatalf("unexpected error: %s", diags.Error())
				}
				if p.client == nil {
					t.Fatal("expected client to be set after successful Configure")
				}
			}
		})
	}
}

func TestConfigure_basic_auth_success(t *testing.T) {
	srv := newAccountAwareServer(t)
	defer srv.Close()

	p := &Provider{handlers: map[string]resourceHandler{}}
	cfg := configMap("endpoint", srv.URL, "auth", "basic", "username", "admin", "password", "secret")

	diags := p.Configure(context.Background(), cfg)
	if diags.HasErrors() {
		t.Fatalf("unexpected error: %s", diags.Error())
	}
	if p.client == nil {
		t.Fatal("expected client to be set after successful Configure")
	}
	if p.caller.UserName != "test" {
		t.Errorf("caller.UserName = %q, want test", p.caller.UserName)
	}
}

func TestConfigure_basic_auth_with_tls_skip_verify(t *testing.T) {
	srv := newAccountAwareServer(t)
	defer srv.Close()

	p := &Provider{handlers: map[string]resourceHandler{}}
	cfg := configMapWithBool("endpoint", srv.URL, "auth", "basic", "username", "admin", "password", "secret", "tls_skip_verify", true)

	diags := p.Configure(context.Background(), cfg)
	if diags.HasErrors() {
		t.Fatalf("unexpected error: %s", diags.Error())
	}
	if p.client == nil {
		t.Fatal("expected client to be set after successful Configure")
	}
}

func TestConfigure_basic_auth_identity_fetch_failure_errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := &Provider{handlers: map[string]resourceHandler{}}
	cfg := configMap("endpoint", srv.URL, "auth", "basic", "username", "admin", "password", "secret")

	diags := p.Configure(context.Background(), cfg)
	if !diags.HasErrors() {
		t.Fatal("Configure should error when /api/account returns 500")
	}
	if !strings.Contains(diags.Error(), "caller identity") {
		t.Errorf("error should mention caller identity, got: %s", diags.Error())
	}
}

func TestProvider_GuardDeletes_flags_role_mapping_and_internal_user(t *testing.T) {
	p := &Provider{
		handlers: map[string]resourceHandler{
			"opensearch_role_mapping":  &roleMappingHandler{},
			"opensearch_internal_user": &internalUserHandler{},
		},
		caller: callerIdentity{UserName: "admin", BackendRoles: []string{"admin"}},
	}

	deletes := []provider.Resource{
		{
			ID: provider.ResourceID{Type: "opensearch_role_mapping", Name: "all_access"},
			Body: buildMap(
				"backend_roles", provider.ListVal([]provider.Value{provider.StringVal("admin")}),
			),
		},
		{
			ID: provider.ResourceID{Type: "opensearch_role_mapping", Name: "readall"},
			Body: buildMap(
				"backend_roles", provider.ListVal([]provider.Value{provider.StringVal("readall")}),
			),
		},
		{ID: provider.ResourceID{Type: "opensearch_internal_user", Name: "admin"}},
		{ID: provider.ResourceID{Type: "opensearch_internal_user", Name: "kibanaro"}},
		{ID: provider.ResourceID{Type: "opensearch_ism_policy", Name: "hot_delete"}},
	}

	guards, diags := p.GuardDeletes(context.Background(), deletes)
	if diags.HasErrors() {
		t.Fatalf("GuardDeletes: %s", diags.Error())
	}

	want := map[string]bool{
		"opensearch_role_mapping.all_access": true,
		"opensearch_internal_user.admin":     true,
	}
	if len(guards) != len(want) {
		t.Fatalf("got %d guards, want %d: %+v", len(guards), len(want), guards)
	}
	for _, g := range guards {
		key := g.Resource.String()
		if !want[key] {
			t.Errorf("unexpected guard on %s", key)
		}
		if g.Reason == "" {
			t.Errorf("guard on %s has empty reason", key)
		}
	}
}

// ─── TypeOrderer ────────────────────────────────────────────────────

func TestTypeOrderer_implements_interface(t *testing.T) {
	var p provider.Provider = &Provider{}
	if _, ok := p.(provider.TypeOrderer); !ok {
		t.Fatal("Provider does not implement provider.TypeOrderer")
	}
}

func TestTypeOrderer_returns_orderings(t *testing.T) {
	p := &Provider{}
	orderings := p.TypeOrderings()

	if got := len(orderings); got != 3 {
		t.Fatalf("expected 3 orderings, got %d", got)
	}

	want := []provider.TypeOrdering{
		{Before: "opensearch_role", After: "opensearch_role_mapping"},
		{Before: "opensearch_internal_user", After: "opensearch_role_mapping"},
		{Before: "opensearch_component_template", After: "opensearch_composable_index_template"},
	}
	for i, w := range want {
		if orderings[i] != w {
			t.Errorf("ordering[%d] = %+v, want %+v", i, orderings[i], w)
		}
	}
}

// ─── Dispatch routing ───────────────────────────────────────────────

func TestNormalize_unknown_type(t *testing.T) {
	p := &Provider{handlers: map[string]resourceHandler{}}
	r := provider.Resource{ID: provider.ResourceID{Type: "opensearch_nonexistent", Name: "x"}}
	_, diags := p.Normalize(context.Background(), r)
	if !diags.HasErrors() {
		t.Fatal("expected error for unknown resource type")
	}
	if msg := diags.Error(); !strings.Contains(msg, "not supported") {
		t.Errorf("expected 'not supported' in error, got: %s", msg)
	}
}

func TestValidate_unknown_type(t *testing.T) {
	p := &Provider{handlers: map[string]resourceHandler{}}
	r := provider.Resource{ID: provider.ResourceID{Type: "opensearch_nonexistent", Name: "x"}}
	diags := p.Validate(context.Background(), r)
	if !diags.HasErrors() {
		t.Fatal("expected error for unknown resource type")
	}
	if msg := diags.Error(); !strings.Contains(msg, "not supported") {
		t.Errorf("expected 'not supported' in error, got: %s", msg)
	}
}

func TestApply_unknown_type(t *testing.T) {
	p := &Provider{handlers: map[string]resourceHandler{}}
	r := provider.Resource{ID: provider.ResourceID{Type: "opensearch_nonexistent", Name: "x"}}
	diags := p.Apply(context.Background(), provider.OpCreate, r)
	if !diags.HasErrors() {
		t.Fatal("expected error for unknown resource type")
	}
	if msg := diags.Error(); !strings.Contains(msg, "not supported") {
		t.Errorf("expected 'not supported' in error, got: %s", msg)
	}
}
