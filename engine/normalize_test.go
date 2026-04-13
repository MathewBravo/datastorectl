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
}
