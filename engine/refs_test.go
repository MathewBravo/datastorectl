package engine

import (
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

func buildIndex(resources ...provider.Resource) map[provider.ResourceID]provider.Resource {
	idx := make(map[provider.ResourceID]provider.Resource, len(resources))
	for _, r := range resources {
		idx[r.ID] = r
	}
	return idx
}

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

func TestResolveReferences(t *testing.T) {
	dbResource := provider.Resource{
		ID:   provider.ResourceID{Type: "db", Name: "main"},
		Body: provider.NewOrderedMap(),
	}
	cacheResource := provider.Resource{
		ID:   provider.ResourceID{Type: "cache", Name: "redis"},
		Body: provider.NewOrderedMap(),
	}
	index := buildIndex(dbResource, cacheResource)

	t.Run("no_references", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("name", provider.StringVal("test"))
		body.Set("count", provider.IntVal(3))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}

		got, err := ResolveReferences(r, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Body.Equal(body) {
			t.Errorf("body changed unexpectedly")
		}
	})

	t.Run("single_ref", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("host", provider.RefVal([]string{"db", "main"}))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}

		got, err := ResolveReferences(r, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		v, _ := got.Body.Get("host")
		if v.Kind != provider.KindString || v.Str != "main" {
			t.Errorf("host = %v, want StringVal(main)", v)
		}
	})

	t.Run("multiple_refs", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("db_host", provider.RefVal([]string{"db", "main"}))
		body.Set("cache_host", provider.RefVal([]string{"cache", "redis"}))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}

		got, err := ResolveReferences(r, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		dbHost, _ := got.Body.Get("db_host")
		if dbHost.Kind != provider.KindString || dbHost.Str != "main" {
			t.Errorf("db_host = %v, want StringVal(main)", dbHost)
		}
		cacheHost, _ := got.Body.Get("cache_host")
		if cacheHost.Kind != provider.KindString || cacheHost.Str != "redis" {
			t.Errorf("cache_host = %v, want StringVal(redis)", cacheHost)
		}
	})

	t.Run("preserves_non_ref_attributes", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("name", provider.StringVal("myapp"))
		body.Set("replicas", provider.IntVal(3))
		body.Set("host", provider.RefVal([]string{"db", "main"}))
		body.Set("enabled", provider.BoolVal(true))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}

		got, err := ResolveReferences(r, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		name, _ := got.Body.Get("name")
		if name.Kind != provider.KindString || name.Str != "myapp" {
			t.Errorf("name = %v, want StringVal(myapp)", name)
		}
		replicas, _ := got.Body.Get("replicas")
		if replicas.Kind != provider.KindInt || replicas.Int != 3 {
			t.Errorf("replicas = %v, want IntVal(3)", replicas)
		}
		enabled, _ := got.Body.Get("enabled")
		if enabled.Kind != provider.KindBool || !enabled.Bool {
			t.Errorf("enabled = %v, want BoolVal(true)", enabled)
		}
	})

	t.Run("ref_in_list", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("hosts", provider.ListVal([]provider.Value{
			provider.StringVal("static"),
			provider.RefVal([]string{"db", "main"}),
		}))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}

		got, err := ResolveReferences(r, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		list, _ := got.Body.Get("hosts")
		if list.Kind != provider.KindList || len(list.List) != 2 {
			t.Fatalf("hosts = %v, want list of 2", list)
		}
		if list.List[0].Str != "static" {
			t.Errorf("list[0] = %v, want static", list.List[0])
		}
		if list.List[1].Kind != provider.KindString || list.List[1].Str != "main" {
			t.Errorf("list[1] = %v, want StringVal(main)", list.List[1])
		}
	})

	t.Run("ref_in_nested_map", func(t *testing.T) {
		inner := provider.NewOrderedMap()
		inner.Set("endpoint", provider.RefVal([]string{"cache", "redis"}))
		body := provider.NewOrderedMap()
		body.Set("config", provider.MapVal(inner))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}

		got, err := ResolveReferences(r, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		config, _ := got.Body.Get("config")
		if config.Kind != provider.KindMap {
			t.Fatalf("config kind = %v, want map", config.Kind)
		}
		endpoint, _ := config.Map.Get("endpoint")
		if endpoint.Kind != provider.KindString || endpoint.Str != "redis" {
			t.Errorf("endpoint = %v, want StringVal(redis)", endpoint)
		}
	})

	t.Run("ref_in_function_arg", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("password", provider.FuncCallVal("secret", []provider.Value{
			provider.RefVal([]string{"db", "main"}),
			provider.StringVal("password"),
		}))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}

		got, err := ResolveReferences(r, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pw, _ := got.Body.Get("password")
		if pw.Kind != provider.KindFunctionCall {
			t.Fatalf("password kind = %v, want function_call", pw.Kind)
		}
		if pw.FuncName != "secret" {
			t.Errorf("FuncName = %q, want secret", pw.FuncName)
		}
		if len(pw.FuncArgs) != 2 {
			t.Fatalf("FuncArgs len = %d, want 2", len(pw.FuncArgs))
		}
		if pw.FuncArgs[0].Kind != provider.KindString || pw.FuncArgs[0].Str != "main" {
			t.Errorf("FuncArgs[0] = %v, want StringVal(main)", pw.FuncArgs[0])
		}
		if pw.FuncArgs[1].Kind != provider.KindString || pw.FuncArgs[1].Str != "password" {
			t.Errorf("FuncArgs[1] = %v, want StringVal(password)", pw.FuncArgs[1])
		}
	})

	t.Run("does_not_mutate_original", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("host", provider.RefVal([]string{"db", "main"}))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: body,
		}

		_, err := ResolveReferences(r, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Original must still be a reference.
		v, _ := r.Body.Get("host")
		if v.Kind != provider.KindReference {
			t.Errorf("original host kind = %v, want reference", v.Kind)
		}
	})

	t.Run("nil_body", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: nil,
		}
		got, err := ResolveReferences(r, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Body != nil {
			t.Errorf("body = %v, want nil", got.Body)
		}
		if got.ID != r.ID {
			t.Errorf("ID = %v, want %v", got.ID, r.ID)
		}
	})

	t.Run("empty_body", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "app", Name: "web"},
			Body: provider.NewOrderedMap(),
		}
		got, err := ResolveReferences(r, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Body.Len() != 0 {
			t.Errorf("body len = %d, want 0", got.Body.Len())
		}
	})
}

func TestResolveReferences_Errors(t *testing.T) {
	index := buildIndex() // empty index — no targets

	t.Run("missing_target", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("host", provider.RefVal([]string{"db", "main"}))
		r := provider.Resource{ID: provider.ResourceID{Type: "app", Name: "web"}, Body: body}

		_, err := ResolveReferences(r, index)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "target not found") {
			t.Errorf("error = %q, want target not found", err.Error())
		}
	})

	t.Run("error_includes_attribute_name", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("db_host", provider.RefVal([]string{"db", "main"}))
		r := provider.Resource{ID: provider.ResourceID{Type: "app", Name: "web"}, Body: body}

		_, err := ResolveReferences(r, index)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), `attribute "db_host"`) {
			t.Errorf("error = %q, want attribute name", err.Error())
		}
	})

	t.Run("error_in_nested_list", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("items", provider.ListVal([]provider.Value{
			provider.StringVal("ok"),
			provider.RefVal([]string{"db", "missing"}),
		}))
		r := provider.Resource{ID: provider.ResourceID{Type: "app", Name: "web"}, Body: body}

		_, err := ResolveReferences(r, index)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "list element 1") {
			t.Errorf("error = %q, want list element index", err.Error())
		}
	})

	t.Run("error_in_nested_map", func(t *testing.T) {
		inner := provider.NewOrderedMap()
		inner.Set("endpoint", provider.RefVal([]string{"cache", "missing"}))
		body := provider.NewOrderedMap()
		body.Set("config", provider.MapVal(inner))
		r := provider.Resource{ID: provider.ResourceID{Type: "app", Name: "web"}, Body: body}

		_, err := ResolveReferences(r, index)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), `key "endpoint"`) {
			t.Errorf("error = %q, want nested key name", err.Error())
		}
	})

	t.Run("error_in_function_arg", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("val", provider.FuncCallVal("secret", []provider.Value{
			provider.RefVal([]string{"db", "missing"}),
		}))
		r := provider.Resource{ID: provider.ResourceID{Type: "app", Name: "web"}, Body: body}

		_, err := ResolveReferences(r, index)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), `function "secret" argument 0`) {
			t.Errorf("error = %q, want function arg context", err.Error())
		}
	})

	t.Run("short_ref", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("bad", provider.RefVal([]string{"onlyonepart"}))
		r := provider.Resource{ID: provider.ResourceID{Type: "app", Name: "web"}, Body: body}

		_, err := ResolveReferences(r, index)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "need at least 2") {
			t.Errorf("error = %q, want need at least 2", err.Error())
		}
	})
}

func TestResolveRef(t *testing.T) {
	index := buildIndex() // empty — only testing passthrough kinds

	scalars := []struct {
		name string
		val  provider.Value
	}{
		{"null", provider.NullVal()},
		{"string", provider.StringVal("hello")},
		{"int", provider.IntVal(42)},
		{"float", provider.FloatVal(3.14)},
		{"bool", provider.BoolVal(true)},
	}
	for _, sc := range scalars {
		t.Run("passthrough_"+sc.name, func(t *testing.T) {
			got, err := resolveRef(sc.val, index)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(sc.val) {
				t.Errorf("got %v, want %v", got, sc.val)
			}
		})
	}

	t.Run("empty_list", func(t *testing.T) {
		v := provider.ListVal([]provider.Value{})
		got, err := resolveRef(v, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Kind != provider.KindList || len(got.List) != 0 {
			t.Errorf("got %v, want empty list", got)
		}
	})

	t.Run("empty_map", func(t *testing.T) {
		v := provider.MapVal(provider.NewOrderedMap())
		got, err := resolveRef(v, index)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Kind != provider.KindMap || got.Map.Len() != 0 {
			t.Errorf("got %v, want empty map", got)
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
