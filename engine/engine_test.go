package engine

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// mockEngineProvider implements provider.Provider with configurable behaviour
// for Engine-level tests. Fields left nil use sensible defaults.
type mockEngineProvider struct {
	configureFn func(ctx context.Context, config *provider.OrderedMap) dcl.Diagnostics
	discoverFn  func(ctx context.Context) ([]provider.Resource, dcl.Diagnostics)
	normalizeFn func(ctx context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics)
}

func (m *mockEngineProvider) Configure(ctx context.Context, config *provider.OrderedMap) dcl.Diagnostics {
	if m.configureFn != nil {
		return m.configureFn(ctx, config)
	}
	return nil
}

func (m *mockEngineProvider) Discover(ctx context.Context) ([]provider.Resource, dcl.Diagnostics) {
	if m.discoverFn != nil {
		return m.discoverFn(ctx)
	}
	return nil, nil
}

func (m *mockEngineProvider) Normalize(ctx context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
	if m.normalizeFn != nil {
		return m.normalizeFn(ctx, r)
	}
	return r, nil
}

func (m *mockEngineProvider) Validate(context.Context, provider.Resource) dcl.Diagnostics {
	return nil
}

func (m *mockEngineProvider) Apply(context.Context, provider.Operation, provider.Resource) dcl.Diagnostics {
	return nil
}

// stubSecretResolver satisfies SecretResolver and always returns the path as
// the resolved value.
type stubSecretResolver struct{}

func (stubSecretResolver) Resolve(_ context.Context, _, path string) (string, error) {
	return path, nil
}

// failSecretResolver returns an error for every call.
type failSecretResolver struct{ err error }

func (f failSecretResolver) Resolve(context.Context, string, string) (string, error) {
	return "", f.err
}

// helper: build a dcl.File with resource blocks from type/name pairs.
func makeFile(resources ...provider.ResourceID) *dcl.File {
	blocks := make([]dcl.Block, len(resources))
	for i, r := range resources {
		blocks[i] = dcl.Block{Type: r.Type, Label: r.Name}
	}
	return &dcl.File{Blocks: blocks}
}

// helper: build a dcl.File with resource blocks carrying attributes.
func makeFileWithAttrs(id provider.ResourceID, attrs []dcl.Attribute) *dcl.File {
	return &dcl.File{
		Blocks: []dcl.Block{
			{Type: id.Type, Label: id.Name, Attributes: attrs},
		},
	}
}

func TestEnginePlan(t *testing.T) {
	t.Run("happy_path_creates", func(t *testing.T) {
		mock := &mockEngineProvider{}
		provider.Register("eng1", func() provider.Provider { return mock })

		e := &Engine{SecretResolver: stubSecretResolver{}}
		file := makeFile(
			provider.ResourceID{Type: "eng1_role", Name: "admin"},
			provider.ResourceID{Type: "eng1_policy", Name: "ro"},
		)

		plan, graph, err := e.Plan(context.Background(), file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(plan.Creates()) != 2 {
			t.Errorf("expected 2 creates, got %d", len(plan.Creates()))
		}
		if !graph.HasNode(rid("eng1_role", "admin")) || !graph.HasNode(rid("eng1_policy", "ro")) {
			t.Error("expected both nodes in graph")
		}
	})

	t.Run("happy_path_update_and_noop", func(t *testing.T) {
		bodyDesired := provider.NewOrderedMap()
		bodyDesired.Set("host", provider.StringVal("new-host"))
		bodyLive := provider.NewOrderedMap()
		bodyLive.Set("host", provider.StringVal("old-host"))
		bodySame := provider.NewOrderedMap()
		bodySame.Set("host", provider.StringVal("same"))

		mock := &mockEngineProvider{
			discoverFn: func(context.Context) ([]provider.Resource, dcl.Diagnostics) {
				return []provider.Resource{
					{ID: rid("eng2_svc", "a"), Body: bodyLive},
					{ID: rid("eng2_svc", "b"), Body: bodySame.Clone()},
				}, nil
			},
		}
		provider.Register("eng2", func() provider.Provider { return mock })

		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "eng2_svc", Label: "a", Attributes: []dcl.Attribute{
					{Key: "host", Value: &dcl.LiteralString{Value: "new-host"}},
				}},
				{Type: "eng2_svc", Label: "b", Attributes: []dcl.Attribute{
					{Key: "host", Value: &dcl.LiteralString{Value: "same"}},
				}},
			},
		}

		e := &Engine{SecretResolver: stubSecretResolver{}}
		plan, _, err := e.Plan(context.Background(), file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(plan.Updates()) != 1 {
			t.Errorf("expected 1 update, got %d", len(plan.Updates()))
		}
		noops := 0
		for _, c := range plan.Changes {
			if c.Type == ChangeNoOp {
				noops++
			}
		}
		if noops != 1 {
			t.Errorf("expected 1 no-op, got %d", noops)
		}
	})

	t.Run("happy_path_delete", func(t *testing.T) {
		mock := &mockEngineProvider{
			discoverFn: func(context.Context) ([]provider.Resource, dcl.Diagnostics) {
				return []provider.Resource{
					{ID: rid("eng3_svc", "orphan"), Body: provider.NewOrderedMap()},
				}, nil
			},
		}
		provider.Register("eng3", func() provider.Provider { return mock })

		// Empty desired — the live resource should become a delete.
		file := makeFile(rid("eng3_svc", "placeholder"))
		file.Blocks = file.Blocks[:0] // remove all blocks but keep valid file

		// We need at least one desired resource so ConfigureProviders gets the
		// provider registered. Use a second resource type.
		file.Blocks = append(file.Blocks, dcl.Block{Type: "eng3_svc", Label: "keeper"})

		e := &Engine{SecretResolver: stubSecretResolver{}}
		plan, _, err := e.Plan(context.Background(), file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(plan.Deletes()) != 1 {
			t.Errorf("expected 1 delete, got %d", len(plan.Deletes()))
		}
		if plan.Deletes()[0].ID.Name != "orphan" {
			t.Errorf("expected orphan delete, got %s", plan.Deletes()[0].ID.Name)
		}
	})

	t.Run("graph_has_reference_edges", func(t *testing.T) {
		mock := &mockEngineProvider{}
		provider.Register("eng4", func() provider.Provider { return mock })

		// Resource A references resource B → graph should have edge A→B.
		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "eng4_svc", Label: "a", Attributes: []dcl.Attribute{
					{Key: "dep", Value: &dcl.Reference{Parts: []string{"eng4_svc", "b"}}},
				}},
				{Type: "eng4_svc", Label: "b"},
			},
		}

		e := &Engine{SecretResolver: stubSecretResolver{}}
		_, graph, err := e.Plan(context.Background(), file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		deps := graph.DependsOn(rid("eng4_svc", "a"))
		found := false
		for _, d := range deps {
			if d == rid("eng4_svc", "b") {
				found = true
			}
		}
		if !found {
			t.Error("expected graph edge eng4_svc.a → eng4_svc.b")
		}
	})

	t.Run("normalization_affects_diff", func(t *testing.T) {
		// Normalize uppercases string values. Desired "hello", live "HELLO" →
		// after normalization both are "HELLO" → ChangeNoOp.
		mock := &mockEngineProvider{
			discoverFn: func(context.Context) ([]provider.Resource, dcl.Diagnostics) {
				body := provider.NewOrderedMap()
				body.Set("val", provider.StringVal("HELLO"))
				return []provider.Resource{
					{ID: rid("eng5_svc", "x"), Body: body},
				}, nil
			},
			normalizeFn: func(_ context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
				if r.Body == nil {
					return r, nil
				}
				out := r.Body.Clone()
				for _, k := range out.Keys() {
					v, _ := out.Get(k)
					if v.Kind == provider.KindString {
						out.Set(k, provider.StringVal(strings.ToUpper(v.Str)))
					}
				}
				return provider.Resource{ID: r.ID, Body: out, SourceRange: r.SourceRange}, nil
			},
		}
		provider.Register("eng5", func() provider.Provider { return mock })

		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "eng5_svc", Label: "x", Attributes: []dcl.Attribute{
					{Key: "val", Value: &dcl.LiteralString{Value: "hello"}},
				}},
			},
		}

		e := &Engine{SecretResolver: stubSecretResolver{}}
		plan, _, err := e.Plan(context.Background(), file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if plan.HasChanges() {
			t.Error("expected no changes after normalization equalises values")
		}
	})

	t.Run("discover_deduplication", func(t *testing.T) {
		var calls atomic.Int32
		mock := &mockEngineProvider{
			discoverFn: func(context.Context) ([]provider.Resource, dcl.Diagnostics) {
				calls.Add(1)
				return nil, nil
			},
		}
		provider.Register("eng6", func() provider.Provider { return mock })

		// Two resource types, same provider prefix → same provider instance.
		file := makeFile(
			provider.ResourceID{Type: "eng6_role", Name: "a"},
			provider.ResourceID{Type: "eng6_policy", Name: "b"},
		)

		e := &Engine{SecretResolver: stubSecretResolver{}}
		_, _, err := e.Plan(context.Background(), file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls.Load() != 1 {
			t.Errorf("expected Discover called once, got %d", calls.Load())
		}
	})

	t.Run("discover_extends_providers_map", func(t *testing.T) {
		// Live has a type not present in desired. NormalizeResources should
		// still succeed because discover extends the providers map.
		mock := &mockEngineProvider{
			discoverFn: func(context.Context) ([]provider.Resource, dcl.Diagnostics) {
				return []provider.Resource{
					{ID: rid("eng7_extra", "live"), Body: provider.NewOrderedMap()},
				}, nil
			},
		}
		provider.Register("eng7", func() provider.Provider { return mock })

		file := makeFile(provider.ResourceID{Type: "eng7_svc", Name: "desired"})

		e := &Engine{SecretResolver: stubSecretResolver{}}
		plan, _, err := e.Plan(context.Background(), file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The live-only resource should appear as a delete.
		if len(plan.Deletes()) != 1 {
			t.Errorf("expected 1 delete for live-only resource, got %d", len(plan.Deletes()))
		}
	})

	t.Run("convert_error", func(t *testing.T) {
		e := &Engine{SecretResolver: stubSecretResolver{}}
		_, _, err := e.Plan(context.Background(), nil, nil)
		if err == nil {
			t.Fatal("expected error for nil file")
		}
		if !strings.Contains(err.Error(), "convert") {
			t.Errorf("expected 'convert' in error, got: %s", err.Error())
		}
	})

	t.Run("configure_error", func(t *testing.T) {
		// Resource type with unknown provider prefix (no registration).
		file := makeFile(provider.ResourceID{Type: "eng8unknown_svc", Name: "a"})

		e := &Engine{SecretResolver: stubSecretResolver{}}
		_, _, err := e.Plan(context.Background(), file, nil)
		if err == nil {
			t.Fatal("expected error for unknown provider")
		}
		if !strings.Contains(err.Error(), "configure") {
			t.Errorf("expected 'configure' in error, got: %s", err.Error())
		}
	})

	t.Run("discover_error", func(t *testing.T) {
		mock := &mockEngineProvider{
			discoverFn: func(context.Context) ([]provider.Resource, dcl.Diagnostics) {
				return nil, dcl.Diagnostics{
					{Severity: dcl.SeverityError, Message: "connection refused"},
				}
			},
		}
		provider.Register("eng9", func() provider.Provider { return mock })

		file := makeFile(provider.ResourceID{Type: "eng9_svc", Name: "a"})

		e := &Engine{SecretResolver: stubSecretResolver{}}
		_, _, err := e.Plan(context.Background(), file, nil)
		if err == nil {
			t.Fatal("expected error from discover")
		}
		if !strings.Contains(err.Error(), "discover") {
			t.Errorf("expected 'discover' in error, got: %s", err.Error())
		}
	})

	t.Run("resolve_references_error", func(t *testing.T) {
		mock := &mockEngineProvider{}
		provider.Register("eng10", func() provider.Provider { return mock })

		// A 1-part reference passes BuildDependencyGraph (which requires ≥2
		// parts to collect) but fails at ResolveReferences.
		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "eng10_svc", Label: "a", Attributes: []dcl.Attribute{
					{Key: "ref", Value: &dcl.Reference{Parts: []string{"single"}}},
				}},
			},
		}

		e := &Engine{SecretResolver: stubSecretResolver{}}
		_, _, err := e.Plan(context.Background(), file, nil)
		if err == nil {
			t.Fatal("expected error for malformed reference")
		}
		if !strings.Contains(err.Error(), "resolve references") {
			t.Errorf("expected 'resolve references' in error, got: %s", err.Error())
		}
	})

	t.Run("resolve_secrets_error", func(t *testing.T) {
		mock := &mockEngineProvider{}
		provider.Register("eng11", func() provider.Provider { return mock })

		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "eng11_svc", Label: "a", Attributes: []dcl.Attribute{
					{Key: "pass", Value: &dcl.FunctionCall{
						Name: "secret",
						Args: []dcl.Expression{
							&dcl.LiteralString{Value: "vault"},
							&dcl.LiteralString{Value: "db/pass"},
						},
					}},
				}},
			},
		}

		e := &Engine{SecretResolver: failSecretResolver{err: errTestFail}}
		_, _, err := e.Plan(context.Background(), file, nil)
		if err == nil {
			t.Fatal("expected error for secret resolution failure")
		}
		if !strings.Contains(err.Error(), "resolve secrets") {
			t.Errorf("expected 'resolve secrets' in error, got: %s", err.Error())
		}
	})

	t.Run("normalize_error", func(t *testing.T) {
		mock := &mockEngineProvider{
			normalizeFn: func(_ context.Context, _ provider.Resource) (provider.Resource, dcl.Diagnostics) {
				return provider.Resource{}, dcl.Diagnostics{
					{Severity: dcl.SeverityError, Message: "bad resource"},
				}
			},
		}
		provider.Register("eng12", func() provider.Provider { return mock })

		file := makeFile(provider.ResourceID{Type: "eng12_svc", Name: "a"})

		e := &Engine{SecretResolver: stubSecretResolver{}}
		_, _, err := e.Plan(context.Background(), file, nil)
		if err == nil {
			t.Fatal("expected error from normalize")
		}
		if !strings.Contains(err.Error(), "normalize") {
			t.Errorf("expected 'normalize' in error, got: %s", err.Error())
		}
	})

	t.Run("graph_cycle_error", func(t *testing.T) {
		mock := &mockEngineProvider{}
		provider.Register("eng13", func() provider.Provider { return mock })

		// Two resources reference each other → BuildDependencyGraph should
		// still succeed (it doesn't detect cycles), but the references are
		// circular. Actually, since both references are valid, the graph
		// builds fine. This tests that mutual references produce edges.
		file := &dcl.File{
			Blocks: []dcl.Block{
				{Type: "eng13_svc", Label: "a", Attributes: []dcl.Attribute{
					{Key: "dep", Value: &dcl.Reference{Parts: []string{"eng13_svc", "b"}}},
				}},
				{Type: "eng13_svc", Label: "b", Attributes: []dcl.Attribute{
					{Key: "dep", Value: &dcl.Reference{Parts: []string{"eng13_svc", "a"}}},
				}},
			},
		}

		e := &Engine{SecretResolver: stubSecretResolver{}}
		_, graph, err := e.Plan(context.Background(), file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Both edges should be present.
		depsA := graph.DependsOn(rid("eng13_svc", "a"))
		depsB := graph.DependsOn(rid("eng13_svc", "b"))
		if len(depsA) != 1 || depsA[0] != rid("eng13_svc", "b") {
			t.Errorf("expected a→b edge, got %v", depsA)
		}
		if len(depsB) != 1 || depsB[0] != rid("eng13_svc", "a") {
			t.Errorf("expected b→a edge, got %v", depsB)
		}
	})

	t.Run("empty_file", func(t *testing.T) {
		e := &Engine{SecretResolver: stubSecretResolver{}}
		file := &dcl.File{}

		plan, graph, err := e.Plan(context.Background(), file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(plan.Changes) != 0 {
			t.Errorf("expected 0 changes, got %d", len(plan.Changes))
		}
		if len(graph.Nodes()) != 0 {
			t.Errorf("expected 0 nodes, got %d", len(graph.Nodes()))
		}
	})

	t.Run("context_propagation", func(t *testing.T) {
		type ctxKey struct{}

		var configureCtx, discoverCtx, normalizeCtx context.Context
		mock := &mockEngineProvider{
			configureFn: func(ctx context.Context, _ *provider.OrderedMap) dcl.Diagnostics {
				configureCtx = ctx
				return nil
			},
			discoverFn: func(ctx context.Context) ([]provider.Resource, dcl.Diagnostics) {
				discoverCtx = ctx
				return nil, nil
			},
			normalizeFn: func(ctx context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
				normalizeCtx = ctx
				return r, nil
			},
		}
		provider.Register("eng14", func() provider.Provider { return mock })

		ctx := context.WithValue(context.Background(), ctxKey{}, "propagated")
		file := makeFile(provider.ResourceID{Type: "eng14_svc", Name: "a"})

		e := &Engine{SecretResolver: stubSecretResolver{}}
		_, _, err := e.Plan(ctx, file, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if configureCtx.Value(ctxKey{}) != "propagated" {
			t.Error("context not propagated to Configure")
		}
		if discoverCtx.Value(ctxKey{}) != "propagated" {
			t.Error("context not propagated to Discover")
		}
		if normalizeCtx.Value(ctxKey{}) != "propagated" {
			t.Error("context not propagated to Normalize")
		}
	})
}

// errTestFail is a sentinel error for test assertions.
var errTestFail = fmt.Errorf("test-induced failure")
