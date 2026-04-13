package engine

import (
	"context"
	"strings"
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

	t.Run("multiple_providers", func(t *testing.T) {
		var instanceA, instanceB *mockConfigProvider
		provider.Register("cptest2a", func() provider.Provider {
			instanceA = &mockConfigProvider{}
			return instanceA
		})
		provider.Register("cptest2b", func() provider.Provider {
			instanceB = &mockConfigProvider{}
			return instanceB
		})

		resources := []provider.Resource{
			{ID: rid("cptest2a_role", "x"), Body: provider.NewOrderedMap()},
			{ID: rid("cptest2b_acl", "y"), Body: provider.NewOrderedMap()},
		}

		result, err := ConfigureProviders(context.Background(), resources, map[string]*provider.OrderedMap{})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !instanceA.configured || !instanceB.configured {
			t.Error("expected both providers to be configured")
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(result))
		}
		if result["cptest2a_role"] != instanceA {
			t.Error("expected cptest2a_role to map to instanceA")
		}
		if result["cptest2b_acl"] != instanceB {
			t.Error("expected cptest2b_acl to map to instanceB")
		}
	})

	t.Run("unknown_provider", func(t *testing.T) {
		resources := []provider.Resource{
			{ID: rid("cptest3unknown_thing", "a"), Body: provider.NewOrderedMap()},
		}

		_, err := ConfigureProviders(context.Background(), resources, map[string]*provider.OrderedMap{})

		if err == nil {
			t.Fatal("expected error for unknown provider")
		}
		if !strings.Contains(err.Error(), "cptest3unknown") {
			t.Errorf("expected prefix in error, got: %s", err.Error())
		}
	})

	t.Run("configure_error", func(t *testing.T) {
		provider.Register("cptest4", func() provider.Provider {
			return &mockConfigProvider{
				configureFn: func(_ context.Context, _ *provider.OrderedMap) dcl.Diagnostics {
					return dcl.Diagnostics{{Severity: dcl.SeverityError, Message: "bad config"}}
				},
			}
		})

		resources := []provider.Resource{
			{ID: rid("cptest4_thing", "a"), Body: provider.NewOrderedMap()},
		}

		_, err := ConfigureProviders(context.Background(), resources, map[string]*provider.OrderedMap{})

		if err == nil {
			t.Fatal("expected error for configure failure")
		}
		if !strings.Contains(err.Error(), "cptest4") {
			t.Errorf("expected provider name in error, got: %s", err.Error())
		}
		if !strings.Contains(err.Error(), "bad config") {
			t.Errorf("expected diagnostic message in error, got: %s", err.Error())
		}
	})

	t.Run("nil_config", func(t *testing.T) {
		var instance *mockConfigProvider
		provider.Register("cptest5", func() provider.Provider {
			instance = &mockConfigProvider{}
			return instance
		})

		resources := []provider.Resource{
			{ID: rid("cptest5_thing", "a"), Body: provider.NewOrderedMap()},
		}

		// No config entry for "cptest5" — should pass nil to Configure.
		result, err := ConfigureProviders(context.Background(), resources, map[string]*provider.OrderedMap{})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !instance.configured {
			t.Fatal("expected Configure to be called")
		}
		if instance.configArg != nil {
			t.Error("expected nil config when not in map")
		}
		if result["cptest5_thing"] == nil {
			t.Error("expected provider in result map")
		}
	})

	t.Run("context_passed_through", func(t *testing.T) {
		var instance *mockConfigProvider
		provider.Register("cptest6", func() provider.Provider {
			instance = &mockConfigProvider{}
			return instance
		})

		type ctxKey struct{}
		ctx := context.WithValue(context.Background(), ctxKey{}, "test-value")

		resources := []provider.Resource{
			{ID: rid("cptest6_thing", "a"), Body: provider.NewOrderedMap()},
		}

		_, err := ConfigureProviders(ctx, resources, map[string]*provider.OrderedMap{})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if instance.configCtx.Value(ctxKey{}) != "test-value" {
			t.Error("expected context to be passed through to Configure")
		}
	})
}
