package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// stubResolver is a map-based SecretResolver keyed by "backend/path".
type stubResolver struct {
	secrets map[string]string
}

func (s stubResolver) Resolve(_ context.Context, backend, path string) (string, error) {
	key := backend + "/" + path
	v, ok := s.secrets[key]
	if !ok {
		return "", fmt.Errorf("stub: secret %q not found", key)
	}
	return v, nil
}

func secret(backend, path string) provider.Value {
	return provider.FuncCallVal("secret", []provider.Value{
		provider.StringVal(backend),
		provider.StringVal(path),
	})
}

func TestResolveSecrets(t *testing.T) {
	ctx := context.Background()
	resolver := stubResolver{secrets: map[string]string{
		"env/DB_PASS":  "hunter2",
		"env/API_KEY":  "abc123",
		"env/NESTED":   "nested-val",
		"env/DEEP_SEC": "deep-secret",
	}}

	t.Run("no_functions", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("name", provider.StringVal("test"))
		body.Set("count", provider.IntVal(3))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "index", Name: "logs"},
			Body: body,
		}

		got, err := ResolveSecrets(ctx, r, resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Body.Equal(body) {
			t.Errorf("body changed unexpectedly")
		}
	})

	t.Run("single_secret", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("password", secret("env", "DB_PASS"))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "index", Name: "logs"},
			Body: body,
		}

		got, err := ResolveSecrets(ctx, r, resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		v, _ := got.Body.Get("password")
		if v.Kind != provider.KindString || v.Str != "hunter2" {
			t.Errorf("password = %v, want StringVal(hunter2)", v)
		}
	})

	t.Run("multiple_secrets", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("password", secret("env", "DB_PASS"))
		body.Set("api_key", secret("env", "API_KEY"))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "index", Name: "logs"},
			Body: body,
		}

		got, err := ResolveSecrets(ctx, r, resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pw, _ := got.Body.Get("password")
		if pw.Kind != provider.KindString || pw.Str != "hunter2" {
			t.Errorf("password = %v, want StringVal(hunter2)", pw)
		}
		ak, _ := got.Body.Get("api_key")
		if ak.Kind != provider.KindString || ak.Str != "abc123" {
			t.Errorf("api_key = %v, want StringVal(abc123)", ak)
		}
	})

	t.Run("preserves_non_secret_attributes", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("name", provider.StringVal("logs"))
		body.Set("replicas", provider.IntVal(3))
		body.Set("password", secret("env", "DB_PASS"))
		body.Set("enabled", provider.BoolVal(true))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "index", Name: "logs"},
			Body: body,
		}

		got, err := ResolveSecrets(ctx, r, resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		name, _ := got.Body.Get("name")
		if name.Kind != provider.KindString || name.Str != "logs" {
			t.Errorf("name = %v, want StringVal(logs)", name)
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

	t.Run("secret_in_list", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("keys", provider.ListVal([]provider.Value{
			provider.StringVal("static"),
			secret("env", "API_KEY"),
		}))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "index", Name: "logs"},
			Body: body,
		}

		got, err := ResolveSecrets(ctx, r, resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		list, _ := got.Body.Get("keys")
		if list.Kind != provider.KindList || len(list.List) != 2 {
			t.Fatalf("keys = %v, want list of 2", list)
		}
		if list.List[0].Str != "static" {
			t.Errorf("list[0] = %v, want static", list.List[0])
		}
		if list.List[1].Kind != provider.KindString || list.List[1].Str != "abc123" {
			t.Errorf("list[1] = %v, want StringVal(abc123)", list.List[1])
		}
	})

	t.Run("secret_in_nested_map", func(t *testing.T) {
		inner := provider.NewOrderedMap()
		inner.Set("token", secret("env", "NESTED"))
		body := provider.NewOrderedMap()
		body.Set("auth", provider.MapVal(inner))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "index", Name: "logs"},
			Body: body,
		}

		got, err := ResolveSecrets(ctx, r, resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		auth, _ := got.Body.Get("auth")
		if auth.Kind != provider.KindMap {
			t.Fatalf("auth kind = %v, want map", auth.Kind)
		}
		token, _ := auth.Map.Get("token")
		if token.Kind != provider.KindString || token.Str != "nested-val" {
			t.Errorf("token = %v, want StringVal(nested-val)", token)
		}
	})

	t.Run("deeply_nested", func(t *testing.T) {
		// map > list > map > secret
		deepMap := provider.NewOrderedMap()
		deepMap.Set("password", secret("env", "DEEP_SEC"))
		listElem := provider.MapVal(deepMap)
		outerMap := provider.NewOrderedMap()
		outerMap.Set("items", provider.ListVal([]provider.Value{listElem}))
		body := provider.NewOrderedMap()
		body.Set("settings", provider.MapVal(outerMap))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "index", Name: "logs"},
			Body: body,
		}

		got, err := ResolveSecrets(ctx, r, resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		settings, _ := got.Body.Get("settings")
		items, _ := settings.Map.Get("items")
		elem := items.List[0]
		pw, _ := elem.Map.Get("password")
		if pw.Kind != provider.KindString || pw.Str != "deep-secret" {
			t.Errorf("deeply nested password = %v, want StringVal(deep-secret)", pw)
		}
	})

	t.Run("does_not_mutate_original", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("password", secret("env", "DB_PASS"))
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "index", Name: "logs"},
			Body: body,
		}

		_, err := ResolveSecrets(ctx, r, resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Original must still be a function call.
		v, _ := r.Body.Get("password")
		if v.Kind != provider.KindFunctionCall {
			t.Errorf("original password kind = %v, want function_call", v.Kind)
		}
	})

	t.Run("nil_body", func(t *testing.T) {
		r := provider.Resource{
			ID:   provider.ResourceID{Type: "index", Name: "logs"},
			Body: nil,
		}
		got, err := ResolveSecrets(ctx, r, resolver)
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
}

func TestResolveSecrets_Errors(t *testing.T) {
	ctx := context.Background()
	resolver := stubResolver{secrets: map[string]string{}}

	t.Run("unsupported_function", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("val", provider.FuncCallVal("unknown", []provider.Value{
			provider.StringVal("a"),
		}))
		r := provider.Resource{ID: provider.ResourceID{Type: "x", Name: "y"}, Body: body}

		_, err := ResolveSecrets(ctx, r, resolver)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), `unsupported function "unknown"`) {
			t.Errorf("error = %q, want unsupported function", err.Error())
		}
	})

	t.Run("wrong_arg_count_0", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("val", provider.FuncCallVal("secret", nil))
		r := provider.Resource{ID: provider.ResourceID{Type: "x", Name: "y"}, Body: body}

		_, err := ResolveSecrets(ctx, r, resolver)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "got 0") {
			t.Errorf("error = %q, want arg count", err.Error())
		}
	})

	t.Run("wrong_arg_count_1", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("val", provider.FuncCallVal("secret", []provider.Value{
			provider.StringVal("env"),
		}))
		r := provider.Resource{ID: provider.ResourceID{Type: "x", Name: "y"}, Body: body}

		_, err := ResolveSecrets(ctx, r, resolver)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "got 1") {
			t.Errorf("error = %q, want arg count", err.Error())
		}
	})

	t.Run("wrong_arg_count_3", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("val", provider.FuncCallVal("secret", []provider.Value{
			provider.StringVal("env"),
			provider.StringVal("KEY"),
			provider.StringVal("extra"),
		}))
		r := provider.Resource{ID: provider.ResourceID{Type: "x", Name: "y"}, Body: body}

		_, err := ResolveSecrets(ctx, r, resolver)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "got 3") {
			t.Errorf("error = %q, want arg count", err.Error())
		}
	})

	t.Run("non_string_arg", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("val", provider.FuncCallVal("secret", []provider.Value{
			provider.IntVal(42),
			provider.StringVal("KEY"),
		}))
		r := provider.Resource{ID: provider.ResourceID{Type: "x", Name: "y"}, Body: body}

		_, err := ResolveSecrets(ctx, r, resolver)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "argument 0 must be a string, got int") {
			t.Errorf("error = %q, want kind name", err.Error())
		}
	})

	t.Run("resolver_error", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("val", secret("env", "MISSING"))
		r := provider.Resource{ID: provider.ResourceID{Type: "x", Name: "y"}, Body: body}

		_, err := ResolveSecrets(ctx, r, resolver)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), `secret("env", "MISSING")`) {
			t.Errorf("error = %q, want secret context", err.Error())
		}
	})

	t.Run("error_includes_attribute_name", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("db_password", secret("env", "MISSING"))
		r := provider.Resource{ID: provider.ResourceID{Type: "x", Name: "y"}, Body: body}

		_, err := ResolveSecrets(ctx, r, resolver)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), `attribute "db_password"`) {
			t.Errorf("error = %q, want attribute name", err.Error())
		}
	})

	t.Run("error_in_nested_list", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("items", provider.ListVal([]provider.Value{
			provider.StringVal("ok"),
			secret("env", "MISSING"),
		}))
		r := provider.Resource{ID: provider.ResourceID{Type: "x", Name: "y"}, Body: body}

		_, err := ResolveSecrets(ctx, r, resolver)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "list element 1") {
			t.Errorf("error = %q, want list element index", err.Error())
		}
	})

	t.Run("error_in_nested_map", func(t *testing.T) {
		inner := provider.NewOrderedMap()
		inner.Set("token", secret("env", "MISSING"))
		body := provider.NewOrderedMap()
		body.Set("auth", provider.MapVal(inner))
		r := provider.Resource{ID: provider.ResourceID{Type: "x", Name: "y"}, Body: body}

		_, err := ResolveSecrets(ctx, r, resolver)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), `key "token"`) {
			t.Errorf("error = %q, want nested key name", err.Error())
		}
	})
}

func TestEnvSecretResolver(t *testing.T) {
	ctx := context.Background()
	r := EnvSecretResolver{}

	t.Run("reads_env_var", func(t *testing.T) {
		t.Setenv("TEST_SECRET", "myvalue")
		v, err := r.Resolve(ctx, "env", "TEST_SECRET")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "myvalue" {
			t.Errorf("got %q, want %q", v, "myvalue")
		}
	})

	t.Run("empty_string_is_valid", func(t *testing.T) {
		t.Setenv("TEST_EMPTY", "")
		v, err := r.Resolve(ctx, "env", "TEST_EMPTY")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "" {
			t.Errorf("got %q, want empty", v)
		}
	})

	t.Run("unset_var_returns_error", func(t *testing.T) {
		_, err := r.Resolve(ctx, "env", "DEFINITELY_NOT_SET_12345")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "DEFINITELY_NOT_SET_12345") {
			t.Errorf("error = %q, want variable name", err.Error())
		}
	})

	t.Run("unsupported_backend", func(t *testing.T) {
		_, err := r.Resolve(ctx, "vault", "some/path")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), `unsupported secret backend "vault"`) {
			t.Errorf("error = %q, want unsupported backend", err.Error())
		}
	})
}

func TestResolveValue(t *testing.T) {
	ctx := context.Background()
	resolver := stubResolver{secrets: map[string]string{}}

	scalars := []struct {
		name string
		val  provider.Value
	}{
		{"null", provider.NullVal()},
		{"string", provider.StringVal("hello")},
		{"int", provider.IntVal(42)},
		{"float", provider.FloatVal(3.14)},
		{"bool", provider.BoolVal(true)},
		{"reference", provider.RefVal([]string{"db", "host"})},
	}
	for _, sc := range scalars {
		t.Run("passthrough_"+sc.name, func(t *testing.T) {
			got, err := resolveValue(ctx, sc.val, resolver)
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
		got, err := resolveValue(ctx, v, resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Kind != provider.KindList || len(got.List) != 0 {
			t.Errorf("got %v, want empty list", got)
		}
	})

	t.Run("empty_map", func(t *testing.T) {
		v := provider.MapVal(provider.NewOrderedMap())
		got, err := resolveValue(ctx, v, resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Kind != provider.KindMap || got.Map.Len() != 0 {
			t.Errorf("got %v, want empty map", got)
		}
	})
}

func TestResolveSecrets_Integration(t *testing.T) {
	t.Setenv("INT_DB_PASS", "s3cret")
	t.Setenv("INT_TOKEN", "tok-abc")

	ctx := context.Background()
	resolver := EnvSecretResolver{}

	authMap := provider.NewOrderedMap()
	authMap.Set("type", provider.StringVal("basic"))
	authMap.Set("token", secret("env", "INT_TOKEN"))

	body := provider.NewOrderedMap()
	body.Set("name", provider.StringVal("myindex"))
	body.Set("replicas", provider.IntVal(2))
	body.Set("password", secret("env", "INT_DB_PASS"))
	body.Set("auth", provider.MapVal(authMap))

	r := provider.Resource{
		ID:   provider.ResourceID{Type: "index", Name: "logs"},
		Body: body,
	}

	got, err := ResolveSecrets(ctx, r, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all fields.
	name, _ := got.Body.Get("name")
	if name.Str != "myindex" {
		t.Errorf("name = %q, want myindex", name.Str)
	}
	replicas, _ := got.Body.Get("replicas")
	if replicas.Int != 2 {
		t.Errorf("replicas = %d, want 2", replicas.Int)
	}
	pw, _ := got.Body.Get("password")
	if pw.Kind != provider.KindString || pw.Str != "s3cret" {
		t.Errorf("password = %v, want StringVal(s3cret)", pw)
	}
	auth, _ := got.Body.Get("auth")
	if auth.Kind != provider.KindMap {
		t.Fatalf("auth kind = %v, want map", auth.Kind)
	}
	authType, _ := auth.Map.Get("type")
	if authType.Str != "basic" {
		t.Errorf("auth.type = %q, want basic", authType.Str)
	}
	token, _ := auth.Map.Get("token")
	if token.Kind != provider.KindString || token.Str != "tok-abc" {
		t.Errorf("auth.token = %v, want StringVal(tok-abc)", token)
	}
}
