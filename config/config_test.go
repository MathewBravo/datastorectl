package config

import (
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// --- SplitFile tests ---

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
	file := &dcl.File{
		Blocks: []dcl.Block{
			{Type: "opensearch_role", Label: "reader"},
		},
	}

	contexts, resources := SplitFile(file)

	if len(contexts) != 0 {
		t.Errorf("expected 0 contexts, got %d", len(contexts))
	}
	if len(resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(resources))
	}
}

func TestSplitFile_no_resources(t *testing.T) {
	file := &dcl.File{
		Blocks: []dcl.Block{
			{Type: "context", Label: "prod"},
			{Type: "context", Label: "staging"},
		},
	}

	contexts, resources := SplitFile(file)

	if len(contexts) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(contexts))
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resources))
	}
}

// --- ParseContexts tests ---

func TestParseContexts_valid(t *testing.T) {
	blocks := []dcl.Block{
		{
			Type:  "context",
			Label: "prod-opensearch",
			Attributes: []dcl.Attribute{
				{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
				{Key: "endpoint", Value: &dcl.LiteralString{Value: "https://search.example.com:9200"}},
				{Key: "auth", Value: &dcl.LiteralString{Value: "basic"}},
				{Key: "username", Value: &dcl.LiteralString{Value: "admin"}},
			},
		},
	}

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

	// Verify provider is NOT in attrs.
	if _, ok := ctx.Attrs.Get("provider"); ok {
		t.Error("provider should not appear in Attrs")
	}

	// Verify other attrs are present.
	endpoint, ok := ctx.Attrs.Get("endpoint")
	if !ok || endpoint.Str != "https://search.example.com:9200" {
		t.Errorf("expected endpoint, got %v", endpoint)
	}
	auth, ok := ctx.Attrs.Get("auth")
	if !ok || auth.Str != "basic" {
		t.Errorf("expected auth=basic, got %v", auth)
	}
	username, ok := ctx.Attrs.Get("username")
	if !ok || username.Str != "admin" {
		t.Errorf("expected username=admin, got %v", username)
	}
}

func TestParseContexts_multiple(t *testing.T) {
	blocks := []dcl.Block{
		{
			Type:  "context",
			Label: "prod-os",
			Attributes: []dcl.Attribute{
				{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
				{Key: "endpoint", Value: &dcl.LiteralString{Value: "https://prod:9200"}},
			},
		},
		{
			Type:  "context",
			Label: "staging-os",
			Attributes: []dcl.Attribute{
				{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
				{Key: "endpoint", Value: &dcl.LiteralString{Value: "https://staging:9200"}},
			},
		},
	}

	contexts, err := ParseContexts(blocks)
	if err != nil {
		t.Fatalf("ParseContexts failed: %v", err)
	}
	if len(contexts) != 2 {
		t.Fatalf("expected 2 contexts, got %d", len(contexts))
	}
	if contexts[0].Name != "prod-os" || contexts[1].Name != "staging-os" {
		t.Errorf("got names %q and %q", contexts[0].Name, contexts[1].Name)
	}
}

func TestParseContexts_missing_label(t *testing.T) {
	blocks := []dcl.Block{
		{
			Type:  "context",
			Label: "",
			Attributes: []dcl.Attribute{
				{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
			},
		},
	}

	_, err := ParseContexts(blocks)
	if err == nil {
		t.Fatal("expected error for missing label")
	}
}

func TestParseContexts_missing_provider(t *testing.T) {
	blocks := []dcl.Block{
		{
			Type:  "context",
			Label: "prod",
			Attributes: []dcl.Attribute{
				{Key: "endpoint", Value: &dcl.LiteralString{Value: "https://prod:9200"}},
			},
		},
	}

	_, err := ParseContexts(blocks)
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestParseContexts_duplicate_name(t *testing.T) {
	blocks := []dcl.Block{
		{
			Type:  "context",
			Label: "prod",
			Attributes: []dcl.Attribute{
				{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
			},
		},
		{
			Type:  "context",
			Label: "prod",
			Attributes: []dcl.Attribute{
				{Key: "provider", Value: &dcl.Identifier{Name: "redis"}},
			},
		},
	}

	_, err := ParseContexts(blocks)
	if err == nil {
		t.Fatal("expected error for duplicate context name")
	}
}

func TestParseContexts_provider_not_string(t *testing.T) {
	blocks := []dcl.Block{
		{
			Type:  "context",
			Label: "prod",
			Attributes: []dcl.Attribute{
				{Key: "provider", Value: &dcl.LiteralInt{Value: 42}},
			},
		},
	}

	_, err := ParseContexts(blocks)
	if err == nil {
		t.Fatal("expected error for non-string provider")
	}
}

func TestParseContexts_provider_as_string_literal(t *testing.T) {
	blocks := []dcl.Block{
		{
			Type:  "context",
			Label: "prod",
			Attributes: []dcl.Attribute{
				{Key: "provider", Value: &dcl.LiteralString{Value: "opensearch"}},
				{Key: "endpoint", Value: &dcl.LiteralString{Value: "https://prod:9200"}},
			},
		},
	}

	contexts, err := ParseContexts(blocks)
	if err != nil {
		t.Fatalf("ParseContexts failed: %v", err)
	}
	if contexts[0].Provider != "opensearch" {
		t.Errorf("expected provider %q, got %q", "opensearch", contexts[0].Provider)
	}
}

func TestParseContexts_converts_secret_calls(t *testing.T) {
	blocks := []dcl.Block{
		{
			Type:  "context",
			Label: "prod",
			Attributes: []dcl.Attribute{
				{Key: "provider", Value: &dcl.Identifier{Name: "opensearch"}},
				{Key: "password", Value: &dcl.FunctionCall{
					Name: "secret",
					Args: []dcl.Expression{
						&dcl.LiteralString{Value: "env"},
						&dcl.LiteralString{Value: "OS_PASSWORD"},
					},
				}},
			},
		},
	}

	contexts, err := ParseContexts(blocks)
	if err != nil {
		t.Fatalf("ParseContexts failed: %v", err)
	}

	pw, ok := contexts[0].Attrs.Get("password")
	if !ok {
		t.Fatal("expected password attr")
	}
	// Should be preserved as a FuncCallVal for later resolution.
	if pw.Kind != provider.KindFunctionCall {
		t.Errorf("expected KindFunctionCall, got %s", pw.Kind)
	}
	if pw.FuncName != "secret" {
		t.Errorf("expected function name %q, got %q", "secret", pw.FuncName)
	}
}
