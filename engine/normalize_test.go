package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// mockNormProvider implements provider.Provider with a configurable Normalize func.
type mockNormProvider struct {
	normalizeFn func(ctx context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics)
}

func (m *mockNormProvider) Configure(context.Context, *provider.OrderedMap) dcl.Diagnostics {
	return nil
}
func (m *mockNormProvider) Discover(context.Context) ([]provider.Resource, dcl.Diagnostics) {
	return nil, nil
}
func (m *mockNormProvider) Normalize(ctx context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
	if m.normalizeFn != nil {
		return m.normalizeFn(ctx, r)
	}
	return r, nil
}
func (m *mockNormProvider) Validate(context.Context, provider.Resource) dcl.Diagnostics { return nil }
func (m *mockNormProvider) Apply(context.Context, provider.Operation, provider.Resource) dcl.Diagnostics {
	return nil
}

func TestNormalizeResources(t *testing.T) {
	t.Run("all_succeed", func(t *testing.T) {
		// Mock normalizer uppercases all string values in body.
		mock := &mockNormProvider{normalizeFn: func(_ context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
			out := provider.Resource{ID: r.ID, Body: provider.NewOrderedMap(), SourceRange: r.SourceRange}
			for _, key := range r.Body.Keys() {
				v, _ := r.Body.Get(key)
				if v.Kind == provider.KindString {
					v.Str = strings.ToUpper(v.Str)
				}
				out.Body.Set(key, v)
			}
			return out, nil
		}}

		resA := provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
		resA.Body.Set("name", provider.StringVal("hello"))
		resB := provider.Resource{ID: rid("svc", "b"), Body: provider.NewOrderedMap()}
		resB.Body.Set("name", provider.StringVal("world"))

		input := []provider.Resource{resA, resB}
		result, err := NormalizeResources(context.Background(), input, map[string]provider.Provider{"svc": mock})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 results, got %d", len(result))
		}

		// Output should have uppercased values.
		v0, _ := result[0].Body.Get("name")
		if v0.Str != "HELLO" {
			t.Errorf("result[0] name: expected HELLO, got %s", v0.Str)
		}
		v1, _ := result[1].Body.Get("name")
		if v1.Str != "WORLD" {
			t.Errorf("result[1] name: expected WORLD, got %s", v1.Str)
		}

		// Input should be unchanged.
		orig0, _ := input[0].Body.Get("name")
		if orig0.Str != "hello" {
			t.Errorf("input[0] should be unchanged, got %s", orig0.Str)
		}
	})

	t.Run("missing_provider", func(t *testing.T) {
		res := provider.Resource{ID: rid("unknown", "a"), Body: provider.NewOrderedMap()}

		_, err := NormalizeResources(context.Background(), []provider.Resource{res}, map[string]provider.Provider{})

		if err == nil {
			t.Fatal("expected error for missing provider")
		}
		if !strings.Contains(err.Error(), "no provider") {
			t.Errorf("expected 'no provider' in error, got: %s", err.Error())
		}
		if !strings.Contains(err.Error(), "unknown") {
			t.Errorf("expected type name in error, got: %s", err.Error())
		}
	})

	t.Run("normalize_error", func(t *testing.T) {
		mock := &mockNormProvider{normalizeFn: func(_ context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
			return r, dcl.Diagnostics{{Severity: dcl.SeverityError, Message: "bad value"}}
		}}

		res := provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}

		_, err := NormalizeResources(context.Background(), []provider.Resource{res}, map[string]provider.Provider{"svc": mock})

		if err == nil {
			t.Fatal("expected error for normalization failure")
		}
		if !strings.Contains(err.Error(), "svc.a") {
			t.Errorf("expected resource ID in error, got: %s", err.Error())
		}
		if !strings.Contains(err.Error(), "bad value") {
			t.Errorf("expected diagnostic message in error, got: %s", err.Error())
		}
	})

	t.Run("empty_input", func(t *testing.T) {
		result, err := NormalizeResources(context.Background(), nil, map[string]provider.Provider{})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d", len(result))
		}
	})

	t.Run("preserves_order", func(t *testing.T) {
		mock := &mockNormProvider{} // returns resource unchanged

		resources := []provider.Resource{
			{ID: rid("svc", "first"), Body: provider.NewOrderedMap()},
			{ID: rid("svc", "second"), Body: provider.NewOrderedMap()},
			{ID: rid("svc", "third"), Body: provider.NewOrderedMap()},
		}

		result, err := NormalizeResources(context.Background(), resources, map[string]provider.Provider{"svc": mock})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("expected 3 results, got %d", len(result))
		}
		for i, r := range result {
			if r.ID != resources[i].ID {
				t.Errorf("result[%d]: expected %s, got %s", i, resources[i].ID, r.ID)
			}
		}
	})

	t.Run("context_passed_through", func(t *testing.T) {
		type ctxKey struct{}
		ctx := context.WithValue(context.Background(), ctxKey{}, "test-value")

		var capturedCtx context.Context
		mock := &mockNormProvider{normalizeFn: func(c context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
			capturedCtx = c
			return r, nil
		}}

		res := provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
		_, err := NormalizeResources(ctx, []provider.Resource{res}, map[string]provider.Provider{"svc": mock})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedCtx.Value(ctxKey{}) != "test-value" {
			t.Error("expected context to be passed through to Normalize")
		}
	})
}
