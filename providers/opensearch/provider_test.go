package opensearch

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// helper builds an *OrderedMap from key-value string pairs.
func configMap(kvs ...string) *provider.OrderedMap {
	m := provider.NewOrderedMap()
	for i := 0; i < len(kvs); i += 2 {
		m.Set(kvs[i], provider.StringVal(kvs[i+1]))
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
			errSubstr: `"basic"`,
		},
		{
			name:      "sigv4_not_implemented",
			config:    configMap("endpoint", "https://localhost:9200", "auth", "sigv4"),
			wantErr:   true,
			errSubstr: "not yet implemented",
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
		{
			name:    "basic_auth_success",
			config:  configMap("endpoint", "https://localhost:9200", "auth", "basic", "username", "admin", "password", "secret"),
			wantErr: false,
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
