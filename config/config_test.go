package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// --- SplitFile tests (#114) ---

func TestSplitFile_separates_contexts_and_resources(t *testing.T) {
	file := &dcl.File{
		Blocks: []dcl.Block{
			{Type: "context", Label: "prod"},
			{Type: "opensearch_role", Label: "reader"},
			{Type: "context", Label: "staging"},
			{Type: "opensearch_ism_policy", Label: "lifecycle"},
		},
	}

	contexts, resources := SplitFile(file)

	if len(contexts) != 2 {
		t.Fatalf("expected 2 context blocks, got %d", len(contexts))
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resource blocks, got %d", len(resources))
	}
	if contexts[0].Label != "prod" || contexts[1].Label != "staging" {
		t.Errorf("context labels: got %q and %q", contexts[0].Label, contexts[1].Label)
	}
	if resources[0].Type != "opensearch_role" || resources[1].Type != "opensearch_ism_policy" {
		t.Errorf("resource types: got %q and %q", resources[0].Type, resources[1].Type)
	}
}

func TestSplitFile_no_contexts(t *testing.T) {
	file := &dcl.File{Blocks: []dcl.Block{{Type: "opensearch_role", Label: "reader"}}}
	contexts, resources := SplitFile(file)
	if len(contexts) != 0 {
		t.Errorf("expected 0 contexts, got %d", len(contexts))
	}
	if len(resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(resources))
	}
}

func TestSplitFile_no_resources(t *testing.T) {
	file := &dcl.File{Blocks: []dcl.Block{{Type: "context", Label: "prod"}, {Type: "context", Label: "staging"}}}
	contexts, resources := SplitFile(file)
	if len(contexts) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(contexts))
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resources))
	}
}

// --- ParseContexts tests (#114) ---

func TestParseContexts_valid(t *testing.T) {
	blocks := []dcl.Block{{
		Type:  "context",
		Label: "prod-opensearch",
		Attributes: []dcl.Attribute{
			{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
			{Key: "endpoint", Value: &dcl.LiteralString{Value: "https://search.example.com:9200"}},
			{Key: "auth", Value: &dcl.LiteralString{Value: "basic"}},
			{Key: "username", Value: &dcl.LiteralString{Value: "admin"}},
		},
	}}

	contexts, err := ParseContexts(blocks)
	if err != nil {
		t.Fatalf("ParseContexts failed: %v", err)
	}
	if len(contexts) != 1 {
		t.Fatalf("expected 1 context, got %d", len(contexts))
	}

	ctx := contexts[0]
	if ctx.Name != "prod-opensearch" {
		t.Errorf("expected name %q, got %q", "prod-opensearch", ctx.Name)
	}
	if ctx.Provider != "opensearch" {
		t.Errorf("expected provider %q, got %q", "opensearch", ctx.Provider)
	}
	if _, ok := ctx.Attrs.Get("provider"); ok {
		t.Error("provider should not appear in Attrs")
	}

	endpoint, ok := ctx.Attrs.Get("endpoint")
	if !ok || endpoint.Str != "https://search.example.com:9200" {
		t.Errorf("expected endpoint, got %v", endpoint)
	}
}

func TestParseContexts_multiple(t *testing.T) {
	blocks := []dcl.Block{
		{Type: "context", Label: "prod-os", Attributes: []dcl.Attribute{
			{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
		}},
		{Type: "context", Label: "staging-os", Attributes: []dcl.Attribute{
			{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
		}},
	}
	contexts, err := ParseContexts(blocks)
	if err != nil {
		t.Fatalf("ParseContexts failed: %v", err)
	}
	if len(contexts) != 2 {
		t.Fatalf("expected 2, got %d", len(contexts))
	}
}

func TestParseContexts_missing_label(t *testing.T) {
	blocks := []dcl.Block{{Type: "context", Label: "", Attributes: []dcl.Attribute{
		{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
	}}}
	_, err := ParseContexts(blocks)
	if err == nil {
		t.Fatal("expected error for missing label")
	}
}

func TestParseContexts_missing_provider(t *testing.T) {
	blocks := []dcl.Block{{Type: "context", Label: "prod", Attributes: []dcl.Attribute{
		{Key: "endpoint", Value: &dcl.LiteralString{Value: "https://prod:9200"}},
	}}}
	_, err := ParseContexts(blocks)
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestParseContexts_duplicate_name(t *testing.T) {
	blocks := []dcl.Block{
		{Type: "context", Label: "prod", Attributes: []dcl.Attribute{{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}}}},
		{Type: "context", Label: "prod", Attributes: []dcl.Attribute{{Key: "provider", Value: &dcl.Identifier{Name: "redis"}}}},
	}
	_, err := ParseContexts(blocks)
	if err == nil {
		t.Fatal("expected error for duplicate context name")
	}
}

func TestParseContexts_provider_not_string(t *testing.T) {
	blocks := []dcl.Block{{Type: "context", Label: "prod", Attributes: []dcl.Attribute{
		{Key: "provider", Value: &dcl.LiteralInt{Value: 42}},
	}}}
	_, err := ParseContexts(blocks)
	if err == nil {
		t.Fatal("expected error for non-string provider")
	}
}

func TestParseContexts_provider_as_string_literal(t *testing.T) {
	blocks := []dcl.Block{{Type: "context", Label: "prod", Attributes: []dcl.Attribute{
		{Key: "provider", Value: &dcl.LiteralString{Value: "opensearch"}},
	}}}
	contexts, err := ParseContexts(blocks)
	if err != nil {
		t.Fatalf("ParseContexts failed: %v", err)
	}
	if contexts[0].Provider != "opensearch" {
		t.Errorf("expected provider %q, got %q", "opensearch", contexts[0].Provider)
	}
}

func TestParseContexts_converts_secret_calls(t *testing.T) {
	blocks := []dcl.Block{{Type: "context", Label: "prod", Attributes: []dcl.Attribute{
		{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
		{Key: "password", Value: &dcl.FunctionCall{
			Name: "secret",
			Args: []dcl.Expression{&dcl.LiteralString{Value: "env"}, &dcl.LiteralString{Value: "OS_PASSWORD"}},
		}},
	}}}
	contexts, err := ParseContexts(blocks)
	if err != nil {
		t.Fatalf("ParseContexts failed: %v", err)
	}
	pw, ok := contexts[0].Attrs.Get("password")
	if !ok {
		t.Fatal("expected password attr")
	}
	if pw.Kind != provider.KindFunctionCall || pw.FuncName != "secret" {
		t.Errorf("expected secret FuncCallVal, got %s %q", pw.Kind, pw.FuncName)
	}
}

// --- BuildConfigs tests (#115) ---

func TestBuildConfigs_single_context(t *testing.T) {
	contexts := []Context{{
		Name: "prod", Provider: "opensearch",
		Attrs: buildAttrs("endpoint", provider.StringVal("https://prod:9200"), "auth", provider.StringVal("basic")),
	}}
	configs, err := BuildConfigs(contexts)
	if err != nil {
		t.Fatalf("BuildConfigs failed: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(configs))
	}
	cfg, ok := configs["opensearch"]
	if !ok {
		t.Fatal("expected config for opensearch")
	}
	endpoint, _ := cfg.Get("endpoint")
	if endpoint.Str != "https://prod:9200" {
		t.Errorf("expected endpoint, got %v", endpoint)
	}
}

func TestBuildConfigs_multiple_providers(t *testing.T) {
	contexts := []Context{
		{Name: "os-prod", Provider: "opensearch", Attrs: provider.NewOrderedMap()},
		{Name: "redis-prod", Provider: "redis", Attrs: provider.NewOrderedMap()},
	}
	configs, err := BuildConfigs(contexts)
	if err != nil {
		t.Fatalf("BuildConfigs failed: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2, got %d", len(configs))
	}
}

func TestBuildConfigs_duplicate_provider_error(t *testing.T) {
	contexts := []Context{
		{Name: "prod-os", Provider: "opensearch", Attrs: provider.NewOrderedMap()},
		{Name: "staging-os", Provider: "opensearch", Attrs: provider.NewOrderedMap()},
	}
	_, err := BuildConfigs(contexts)
	if err == nil {
		t.Fatal("expected error for duplicate provider")
	}
}

func TestBuildConfigs_empty(t *testing.T) {
	configs, err := BuildConfigs(nil)
	if err != nil {
		t.Fatalf("BuildConfigs failed: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("expected empty map, got %d", len(configs))
	}
}

// --- ResolveResourceContexts tests (#116) ---

func TestResolveResourceContexts_strips_context(t *testing.T) {
	resources := []provider.Resource{{
		ID: provider.ResourceID{Type: "opensearch_role", Name: "reader"},
		Body: buildAttrs(
			"context", provider.StringVal("prod"),
			"cluster_permissions", provider.ListVal([]provider.Value{provider.StringVal("read")}),
		),
	}}
	contexts := []Context{{Name: "prod", Provider: "opensearch", Attrs: provider.NewOrderedMap()}}

	resolved, err := ResolveResourceContexts(resources, contexts)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if _, ok := resolved[0].Body.Get("context"); ok {
		t.Error("expected context attribute to be stripped")
	}
	if _, ok := resolved[0].Body.Get("cluster_permissions"); !ok {
		t.Error("expected cluster_permissions to be preserved")
	}
}

func TestResolveResourceContexts_unknown_context(t *testing.T) {
	resources := []provider.Resource{{
		ID:   provider.ResourceID{Type: "opensearch_role", Name: "reader"},
		Body: buildAttrs("context", provider.StringVal("nonexistent")),
	}}
	_, err := ResolveResourceContexts(resources, nil)
	if err == nil {
		t.Fatal("expected error for unknown context")
	}
}

func TestResolveResourceContexts_provider_mismatch(t *testing.T) {
	resources := []provider.Resource{{
		ID:   provider.ResourceID{Type: "opensearch_role", Name: "reader"},
		Body: buildAttrs("context", provider.StringVal("redis-prod")),
	}}
	contexts := []Context{{Name: "redis-prod", Provider: "redis", Attrs: provider.NewOrderedMap()}}

	_, err := ResolveResourceContexts(resources, contexts)
	if err == nil {
		t.Fatal("expected error for provider mismatch")
	}
}

func TestResolveResourceContexts_missing_context_attr(t *testing.T) {
	resources := []provider.Resource{{
		ID:   provider.ResourceID{Type: "opensearch_role", Name: "reader"},
		Body: buildAttrs("cluster_permissions", provider.ListVal(nil)),
	}}
	_, err := ResolveResourceContexts(resources, nil)
	if err == nil {
		t.Fatal("expected error for missing context attribute")
	}
}

func TestResolveResourceContexts_multiple_resources(t *testing.T) {
	resources := []provider.Resource{
		{ID: provider.ResourceID{Type: "opensearch_role", Name: "reader"}, Body: buildAttrs("context", provider.StringVal("prod"))},
		{ID: provider.ResourceID{Type: "opensearch_role", Name: "writer"}, Body: buildAttrs("context", provider.StringVal("prod"))},
	}
	contexts := []Context{{Name: "prod", Provider: "opensearch", Attrs: provider.NewOrderedMap()}}

	resolved, err := ResolveResourceContexts(resources, contexts)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("expected 2, got %d", len(resolved))
	}
	for _, r := range resolved {
		if _, ok := r.Body.Get("context"); ok {
			t.Errorf("resource %s still has context attribute", r.ID)
		}
	}
}

// --- LoadConfigFile tests (#118) ---

func TestLoadConfigFile_valid(t *testing.T) {
	contexts, err := LoadConfigFile("testdata/valid_config.dcl")
	if err != nil {
		t.Fatalf("LoadConfigFile failed: %v", err)
	}
	if len(contexts) != 2 {
		t.Fatalf("expected 2 contexts, got %d", len(contexts))
	}
	if contexts[0].Name != "prod" || contexts[1].Name != "staging" {
		t.Errorf("got names %q and %q", contexts[0].Name, contexts[1].Name)
	}
}

func TestLoadConfigFile_rejects_resource_blocks(t *testing.T) {
	_, err := LoadConfigFile("testdata/invalid_has_resources.dcl")
	if err == nil {
		t.Fatal("expected error for resource blocks in config file")
	}
}

func TestLoadConfigFile_missing_file(t *testing.T) {
	contexts, err := LoadConfigFile("/nonexistent/path/config.dcl")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(contexts) != 0 {
		t.Errorf("expected empty slice, got %d contexts", len(contexts))
	}
}

func TestLoadConfigFile_parse_error(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.dcl")
	os.WriteFile(path, []byte("this is not { valid dcl !!!"), 0644)

	_, err := LoadConfigFile(path)
	if err == nil {
		t.Fatal("expected error for malformed DCL")
	}
}

// --- MergeContexts tests (#118) ---

func TestMergeContexts_no_overlap(t *testing.T) {
	inline := []Context{{Name: "prod", Provider: "opensearch"}}
	fromFile := []Context{{Name: "staging", Provider: "opensearch"}}

	merged, err := MergeContexts(inline, fromFile)
	if err != nil {
		t.Fatalf("MergeContexts failed: %v", err)
	}
	if len(merged) != 2 {
		t.Fatalf("expected 2, got %d", len(merged))
	}
}

func TestMergeContexts_duplicate_name_error(t *testing.T) {
	inline := []Context{{Name: "prod", Provider: "opensearch"}}
	fromFile := []Context{{Name: "prod", Provider: "opensearch"}}

	_, err := MergeContexts(inline, fromFile)
	if err == nil {
		t.Fatal("expected error for duplicate context name")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if !strings.HasSuffix(path, filepath.Join(".datastorectl", "config.dcl")) {
		t.Errorf("unexpected path: %s", path)
	}
}

// --- ResolveConfigSecrets tests (#117) ---

// stubResolver implements SecretResolver for testing.
type stubResolver struct{}

func (stubResolver) Resolve(_ context.Context, backend, path string) (string, error) {
	if backend != "env" {
		return "", fmt.Errorf("unsupported secret backend %q", backend)
	}
	v, ok := os.LookupEnv(path)
	if !ok {
		return "", fmt.Errorf("environment variable %q is not set", path)
	}
	return v, nil
}

func TestResolveConfigSecrets_env_resolved(t *testing.T) {
	t.Setenv("TEST_SECRET_VALUE", "my-password")

	configs := map[string]*provider.OrderedMap{
		"opensearch": buildAttrs(
			"endpoint", provider.StringVal("https://prod:9200"),
			"password", provider.FuncCallVal("secret", []provider.Value{
				provider.StringVal("env"),
				provider.StringVal("TEST_SECRET_VALUE"),
			}),
		),
	}

	err := ResolveConfigSecrets(context.Background(), configs, stubResolver{})
	if err != nil {
		t.Fatalf("ResolveConfigSecrets failed: %v", err)
	}

	pw, _ := configs["opensearch"].Get("password")
	if pw.Kind != provider.KindString || pw.Str != "my-password" {
		t.Errorf("expected resolved string \"my-password\", got %s %q", pw.Kind, pw.Str)
	}
}

func TestResolveConfigSecrets_unsupported_backend(t *testing.T) {
	configs := map[string]*provider.OrderedMap{
		"opensearch": buildAttrs(
			"password", provider.FuncCallVal("secret", []provider.Value{
				provider.StringVal("vault"),
				provider.StringVal("path/to/secret"),
			}),
		),
	}

	err := ResolveConfigSecrets(context.Background(), configs, stubResolver{})
	if err == nil {
		t.Fatal("expected error for unsupported backend")
	}
}

func TestResolveConfigSecrets_missing_env_var(t *testing.T) {
	configs := map[string]*provider.OrderedMap{
		"opensearch": buildAttrs(
			"password", provider.FuncCallVal("secret", []provider.Value{
				provider.StringVal("env"),
				provider.StringVal("DEFINITELY_NOT_SET_12345"),
			}),
		),
	}

	err := ResolveConfigSecrets(context.Background(), configs, stubResolver{})
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
}

func TestResolveConfigSecrets_non_secret_function(t *testing.T) {
	configs := map[string]*provider.OrderedMap{
		"opensearch": buildAttrs(
			"value", provider.FuncCallVal("other_func", []provider.Value{
				provider.StringVal("arg"),
			}),
		),
	}

	err := ResolveConfigSecrets(context.Background(), configs, stubResolver{})
	if err == nil {
		t.Fatal("expected error for unsupported function")
	}
}

func TestResolveConfigSecrets_no_secrets(t *testing.T) {
	configs := map[string]*provider.OrderedMap{
		"opensearch": buildAttrs(
			"endpoint", provider.StringVal("https://prod:9200"),
			"auth", provider.StringVal("basic"),
		),
	}

	err := ResolveConfigSecrets(context.Background(), configs, stubResolver{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// --- test helpers ---

func buildAttrs(kvs ...any) *provider.OrderedMap {
	m := provider.NewOrderedMap()
	for i := 0; i < len(kvs); i += 2 {
		m.Set(kvs[i].(string), kvs[i+1].(provider.Value))
	}
	return m
}
