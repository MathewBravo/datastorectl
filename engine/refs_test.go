package engine

import (
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

func TestExtractReferences_Basic(t *testing.T) {
	t.Run("no_references", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("name", provider.StringVal("mydb"))
		body.Set("port", provider.IntVal(5432))

		r := provider.Resource{
			ID:   provider.ResourceID{Type: "postgres", Name: "main"},
			Body: body,
		}
		refs := ExtractReferences(r)
		if len(refs) != 0 {
			t.Fatalf("expected 0 refs, got %d", len(refs))
		}
	})

	t.Run("single_reference", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("host", provider.RefVal([]string{"db", "host"}))

		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}
		refs := ExtractReferences(r)
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref, got %d", len(refs))
		}
		if refs[0].Type != "db" || refs[0].Name != "host" {
			t.Fatalf("expected {db host}, got %+v", refs[0])
		}
	})

	t.Run("multiple_references", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("host", provider.RefVal([]string{"db", "host"}))
		body.Set("port", provider.RefVal([]string{"db", "port"}))

		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}
		refs := ExtractReferences(r)
		if len(refs) != 2 {
			t.Fatalf("expected 2 refs, got %d", len(refs))
		}
		if refs[0].Type != "db" || refs[0].Name != "host" {
			t.Fatalf("expected {db host} at [0], got %+v", refs[0])
		}
		if refs[1].Type != "db" || refs[1].Name != "port" {
			t.Fatalf("expected {db port} at [1], got %+v", refs[1])
		}
	})

	t.Run("nil_body", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: nil,
		}
		refs := ExtractReferences(r)
		if refs == nil {
			t.Fatal("expected non-nil empty slice, got nil")
		}
		if len(refs) != 0 {
			t.Fatalf("expected 0 refs, got %d", len(refs))
		}
	})
}

func TestExtractReferences_Nested(t *testing.T) {
	t.Run("ref_in_list", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("items", provider.ListVal([]provider.Value{
			provider.StringVal("literal"),
			provider.RefVal([]string{"cache", "endpoint"}),
			provider.IntVal(42),
		}))

		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}
		refs := ExtractReferences(r)
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref, got %d", len(refs))
		}
		if refs[0].Type != "cache" || refs[0].Name != "endpoint" {
			t.Fatalf("expected {cache endpoint}, got %+v", refs[0])
		}
	})

	t.Run("ref_in_nested_map", func(t *testing.T) {
		inner := provider.NewOrderedMap()
		inner.Set("url", provider.RefVal([]string{"db", "url"}))

		outer := provider.NewOrderedMap()
		outer.Set("conn", provider.MapVal(inner))

		body := provider.NewOrderedMap()
		body.Set("config", provider.MapVal(outer))

		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}
		refs := ExtractReferences(r)
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref, got %d", len(refs))
		}
		if refs[0].Type != "db" || refs[0].Name != "url" {
			t.Fatalf("expected {db url}, got %+v", refs[0])
		}
	})

	t.Run("ref_in_function_arg", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("password", provider.FuncCallVal("secret", []provider.Value{
			provider.RefVal([]string{"vault", "db_pass"}),
		}))

		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}
		refs := ExtractReferences(r)
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref, got %d", len(refs))
		}
		if refs[0].Type != "vault" || refs[0].Name != "db_pass" {
			t.Fatalf("expected {vault db_pass}, got %+v", refs[0])
		}
	})

	t.Run("mixed_nesting", func(t *testing.T) {
		inner := provider.NewOrderedMap()
		inner.Set("addr", provider.RefVal([]string{"db", "addr"}))

		body := provider.NewOrderedMap()
		body.Set("backends", provider.ListVal([]provider.Value{
			provider.MapVal(inner),
			provider.RefVal([]string{"cache", "endpoint"}),
		}))

		r := provider.Resource{
			ID:   provider.ResourceID{Type: "lb", Name: "main"},
			Body: body,
		}
		refs := ExtractReferences(r)
		if len(refs) != 2 {
			t.Fatalf("expected 2 refs, got %d", len(refs))
		}
		if refs[0].Type != "db" || refs[0].Name != "addr" {
			t.Fatalf("expected {db addr} at [0], got %+v", refs[0])
		}
		if refs[1].Type != "cache" || refs[1].Name != "endpoint" {
			t.Fatalf("expected {cache endpoint} at [1], got %+v", refs[1])
		}
	})
}

func TestExtractReferences_EdgeCases(t *testing.T) {
	t.Run("duplicate_refs_preserved", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("a", provider.RefVal([]string{"db", "host"}))
		body.Set("b", provider.RefVal([]string{"db", "host"}))

		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}
		refs := ExtractReferences(r)
		if len(refs) != 2 {
			t.Fatalf("expected 2 refs (duplicates preserved), got %d", len(refs))
		}
	})

	t.Run("short_ref_skipped", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("bad", provider.RefVal([]string{"onlyonepart"}))

		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}
		refs := ExtractReferences(r)
		if len(refs) != 0 {
			t.Fatalf("expected 0 refs (short ref skipped), got %d", len(refs))
		}
	})

	t.Run("empty_body", func(t *testing.T) {
		body := provider.NewOrderedMap()

		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}
		refs := ExtractReferences(r)
		if len(refs) != 0 {
			t.Fatalf("expected 0 refs, got %d", len(refs))
		}
	})
}
