package engine

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// mockConfigProvider implements provider.Provider with a configurable Configure func.
type mockConfigProvider struct {
	configureFn func(ctx context.Context, config *provider.OrderedMap) dcl.Diagnostics
	configured  bool
	configArg   *provider.OrderedMap
	configCtx   context.Context
}

func (m *mockConfigProvider) Configure(ctx context.Context, config *provider.OrderedMap) dcl.Diagnostics {
	m.configured = true
	m.configArg = config
	m.configCtx = ctx
	if m.configureFn != nil {
		return m.configureFn(ctx, config)
	}
	return nil
}
func (m *mockConfigProvider) Discover(context.Context) ([]provider.Resource, dcl.Diagnostics) {
	return nil, nil
}
func (m *mockConfigProvider) Normalize(_ context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
	return r, nil
}
func (m *mockConfigProvider) Validate(context.Context, provider.Resource) dcl.Diagnostics { return nil }
func (m *mockConfigProvider) Apply(context.Context, provider.Operation, provider.Resource) dcl.Diagnostics {
	return nil
}

func TestConfigureProviders(t *testing.T) {
	t.Run("all_succeed", func(t *testing.T) {
		var instance *mockConfigProvider
		provider.Register("cptest1", func() provider.Provider {
			instance = &mockConfigProvider{}
			return instance
		})

		resources := []provider.Resource{
			{ID: rid("cptest1_role", "a"), Body: provider.NewOrderedMap()},
			{ID: rid("cptest1_policy", "b"), Body: provider.NewOrderedMap()},
		}

		cfg := provider.NewOrderedMap()
		cfg.Set("host", provider.StringVal("localhost"))
		configs := map[string]*provider.OrderedMap{"cptest1": cfg}

		result, err := ConfigureProviders(context.Background(), resources, configs)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !instance.configured {
			t.Fatal("expected Configure to be called")
		}
		if instance.configArg != cfg {
			t.Error("expected config to be passed through")
		}

		// Both resource types should map to the same provider instance.
		p1, ok1 := result["cptest1_role"]
		p2, ok2 := result["cptest1_policy"]
		if !ok1 || !ok2 {
			t.Fatal("expected both resource types in map")
		}
		if p1 != p2 {
			t.Error("expected same provider instance for both types")
		}
	})
}
