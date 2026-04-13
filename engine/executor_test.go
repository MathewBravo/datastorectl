package engine

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// mockProvider implements provider.Provider with a configurable Apply func.
type mockProvider struct {
	applyFn func(ctx context.Context, op provider.Operation, r provider.Resource) dcl.Diagnostics
}

func (m *mockProvider) Configure(context.Context, *provider.OrderedMap) dcl.Diagnostics { return nil }
func (m *mockProvider) Discover(context.Context) ([]provider.Resource, dcl.Diagnostics) {
	return nil, nil
}
func (m *mockProvider) Normalize(_ context.Context, r provider.Resource) (provider.Resource, dcl.Diagnostics) {
	return r, nil
}
func (m *mockProvider) Validate(context.Context, provider.Resource) dcl.Diagnostics { return nil }
func (m *mockProvider) Apply(ctx context.Context, op provider.Operation, r provider.Resource) dcl.Diagnostics {
	if m.applyFn != nil {
		return m.applyFn(ctx, op, r)
	}
	return nil
}

func errDiags(msg string) dcl.Diagnostics {
	return dcl.Diagnostics{{Severity: dcl.SeverityError, Message: msg}}
}

func dummyResource(typ, name string) *provider.Resource {
	return &provider.Resource{ID: rid(typ, name), Body: provider.NewOrderedMap()}
}

func findResult(results []ResourceResult, id provider.ResourceID) ResourceResult {
	for _, r := range results {
		if r.ID == id {
			return r
		}
	}
	return ResourceResult{}
}

func TestExecute(t *testing.T) {
	t.Run("all_succeed", func(t *testing.T) {
		g := NewGraph()
		a, b := rid("svc", "a"), rid("svc", "b")
		g.AddEdge(a, b)

		plan := &OrderedPlan{
			Layers: [][]ResourceChange{
				{{ID: b, Type: ChangeCreate, Desired: dummyResource("svc", "b")}},
				{{ID: a, Type: ChangeCreate, Desired: dummyResource("svc", "a")}},
			},
		}

		mock := &mockProvider{applyFn: func(_ context.Context, _ provider.Operation, _ provider.Resource) dcl.Diagnostics {
			return nil
		}}

		result := Execute(context.Background(), plan, g, map[string]provider.Provider{"svc": mock})

		if len(result.Results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(result.Results))
		}
		for _, r := range result.Results {
			if r.Status != StatusSuccess {
				t.Errorf("resource %s: expected success, got %s", r.ID, r.Status)
			}
		}
	})

	t.Run("failure_skips_dependent", func(t *testing.T) {
		g := NewGraph()
		a, b := rid("svc", "a"), rid("svc", "b")
		g.AddEdge(a, b)

		plan := &OrderedPlan{
			Layers: [][]ResourceChange{
				{{ID: b, Type: ChangeCreate, Desired: dummyResource("svc", "b")}},
				{{ID: a, Type: ChangeCreate, Desired: dummyResource("svc", "a")}},
			},
		}

		mock := &mockProvider{applyFn: func(_ context.Context, _ provider.Operation, _ provider.Resource) dcl.Diagnostics {
			return errDiags("boom")
		}}

		result := Execute(context.Background(), plan, g, map[string]provider.Provider{"svc": mock})

		rb := findResult(result.Results, b)
		ra := findResult(result.Results, a)
		if rb.Status != StatusFailed {
			t.Errorf("B: expected failed, got %s", rb.Status)
		}
		if ra.Status != StatusSkipped {
			t.Errorf("A: expected skipped, got %s", ra.Status)
		}
	})

	t.Run("transitive_skip", func(t *testing.T) {
		// A→B→C; C fails → B skipped → A skipped.
		g := NewGraph()
		a, b, c := rid("svc", "a"), rid("svc", "b"), rid("svc", "c")
		g.AddEdge(a, b)
		g.AddEdge(b, c)

		plan := &OrderedPlan{
			Layers: [][]ResourceChange{
				{{ID: c, Type: ChangeCreate, Desired: dummyResource("svc", "c")}},
				{{ID: b, Type: ChangeCreate, Desired: dummyResource("svc", "b")}},
				{{ID: a, Type: ChangeCreate, Desired: dummyResource("svc", "a")}},
			},
		}

		mock := &mockProvider{applyFn: func(_ context.Context, _ provider.Operation, _ provider.Resource) dcl.Diagnostics {
			return errDiags("boom")
		}}

		result := Execute(context.Background(), plan, g, map[string]provider.Provider{"svc": mock})

		rc := findResult(result.Results, c)
		rb := findResult(result.Results, b)
		ra := findResult(result.Results, a)
		if rc.Status != StatusFailed {
			t.Errorf("C: expected failed, got %s", rc.Status)
		}
		if rb.Status != StatusSkipped {
			t.Errorf("B: expected skipped, got %s", rb.Status)
		}
		if ra.Status != StatusSkipped {
			t.Errorf("A: expected skipped, got %s", ra.Status)
		}
	})

	t.Run("independent_unaffected_by_failure", func(t *testing.T) {
		g := NewGraph()
		a, b, c := rid("svc", "a"), rid("svc", "b"), rid("svc", "c")
		g.AddEdge(a, b)
		g.AddNode(c)

		plan := &OrderedPlan{
			Layers: [][]ResourceChange{
				{
					{ID: b, Type: ChangeCreate, Desired: dummyResource("svc", "b")},
					{ID: c, Type: ChangeCreate, Desired: dummyResource("svc", "c")},
				},
				{{ID: a, Type: ChangeCreate, Desired: dummyResource("svc", "a")}},
			},
		}

		mock := &mockProvider{applyFn: func(_ context.Context, _ provider.Operation, r provider.Resource) dcl.Diagnostics {
			if r.ID == b {
				return errDiags("boom")
			}
			return nil
		}}

		result := Execute(context.Background(), plan, g, map[string]provider.Provider{"svc": mock})

		rb := findResult(result.Results, b)
		ra := findResult(result.Results, a)
		rc := findResult(result.Results, c)
		if rb.Status != StatusFailed {
			t.Errorf("B: expected failed, got %s", rb.Status)
		}
		if ra.Status != StatusSkipped {
			t.Errorf("A: expected skipped, got %s", ra.Status)
		}
		if rc.Status != StatusSuccess {
			t.Errorf("C: expected success, got %s", rc.Status)
		}
	})

	t.Run("no_ops_skip_apply", func(t *testing.T) {
		g := NewGraph()
		a := rid("svc", "a")
		g.AddNode(a)

		plan := &OrderedPlan{
			Layers: [][]ResourceChange{
				{{ID: a, Type: ChangeNoOp, Desired: dummyResource("svc", "a")}},
			},
		}

		var callCount atomic.Int32
		mock := &mockProvider{applyFn: func(_ context.Context, _ provider.Operation, _ provider.Resource) dcl.Diagnostics {
			callCount.Add(1)
			return nil
		}}

		result := Execute(context.Background(), plan, g, map[string]provider.Provider{"svc": mock})

		if len(result.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result.Results))
		}
		if result.Results[0].Status != StatusSuccess {
			t.Errorf("expected success, got %s", result.Results[0].Status)
		}
		if callCount.Load() != 0 {
			t.Errorf("expected 0 Apply calls for no-op, got %d", callCount.Load())
		}
	})

	t.Run("missing_provider", func(t *testing.T) {
		g := NewGraph()
		a := rid("unknown", "a")
		g.AddNode(a)

		plan := &OrderedPlan{
			Layers: [][]ResourceChange{
				{{ID: a, Type: ChangeCreate, Desired: dummyResource("unknown", "a")}},
			},
		}

		result := Execute(context.Background(), plan, g, map[string]provider.Provider{})

		if len(result.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result.Results))
		}
		r := result.Results[0]
		if r.Status != StatusFailed {
			t.Errorf("expected failed, got %s", r.Status)
		}
		if r.Error == nil || !strings.Contains(r.Error.Error(), "no provider") {
			t.Errorf("expected error about missing provider, got %v", r.Error)
		}
	})

	t.Run("context_cancelled", func(t *testing.T) {
		g := NewGraph()
		a, b := rid("svc", "a"), rid("svc", "b")
		g.AddEdge(a, b)

		plan := &OrderedPlan{
			Layers: [][]ResourceChange{
				{{ID: b, Type: ChangeCreate, Desired: dummyResource("svc", "b")}},
				{{ID: a, Type: ChangeCreate, Desired: dummyResource("svc", "a")}},
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		mock := &mockProvider{}
		result := Execute(ctx, plan, g, map[string]provider.Provider{"svc": mock})

		rb := findResult(result.Results, b)
		ra := findResult(result.Results, a)
		if rb.Status != StatusFailed {
			t.Errorf("B: expected failed, got %s", rb.Status)
		}
		if !errors.Is(rb.Error, context.Canceled) {
			t.Errorf("B: expected context.Canceled error, got %v", rb.Error)
		}
		if ra.Status != StatusSkipped {
			t.Errorf("A: expected skipped, got %s", ra.Status)
		}
	})

	t.Run("delete_uses_live_resource", func(t *testing.T) {
		g := NewGraph()
		a := rid("svc", "a")
		g.AddNode(a)

		live := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
		live.Body.Set("key", provider.StringVal("live-value"))

		plan := &OrderedPlan{
			Layers: [][]ResourceChange{
				{{ID: a, Type: ChangeDelete, Live: live}},
			},
		}

		var capturedOp provider.Operation
		var capturedRes provider.Resource
		mock := &mockProvider{applyFn: func(_ context.Context, op provider.Operation, r provider.Resource) dcl.Diagnostics {
			capturedOp = op
			capturedRes = r
			return nil
		}}

		result := Execute(context.Background(), plan, g, map[string]provider.Provider{"svc": mock})

		if result.Results[0].Status != StatusSuccess {
			t.Errorf("expected success, got %s", result.Results[0].Status)
		}
		if capturedOp != provider.OpDelete {
			t.Errorf("expected OpDelete, got %s", capturedOp)
		}
		if capturedRes.ID != live.ID {
			t.Errorf("expected live resource ID %s, got %s", live.ID, capturedRes.ID)
		}
	})

	t.Run("mixed_operations", func(t *testing.T) {
		// Three independent resources: create B, update C, delete D.
		g := NewGraph()
		b, c, d := rid("svc", "b"), rid("svc", "c"), rid("svc", "d")
		g.AddNode(b)
		g.AddNode(c)
		g.AddNode(d)

		plan := &OrderedPlan{
			Layers: [][]ResourceChange{
				{
					{ID: b, Type: ChangeCreate, Desired: dummyResource("svc", "b")},
					{ID: c, Type: ChangeUpdate, Desired: dummyResource("svc", "c")},
					{ID: d, Type: ChangeDelete, Live: dummyResource("svc", "d")},
				},
			},
		}

		ops := make(map[provider.ResourceID]provider.Operation)
		mock := &mockProvider{applyFn: func(_ context.Context, op provider.Operation, r provider.Resource) dcl.Diagnostics {
			ops[r.ID] = op
			return nil
		}}

		result := Execute(context.Background(), plan, g, map[string]provider.Provider{"svc": mock})

		for _, r := range result.Results {
			if r.Status != StatusSuccess {
				t.Errorf("resource %s: expected success, got %s", r.ID, r.Status)
			}
		}
		if ops[b] != provider.OpCreate {
			t.Errorf("B: expected OpCreate, got %s", ops[b])
		}
		if ops[c] != provider.OpUpdate {
			t.Errorf("C: expected OpUpdate, got %s", ops[c])
		}
		if ops[d] != provider.OpDelete {
			t.Errorf("D: expected OpDelete, got %s", ops[d])
		}
	})

	t.Run("parallel_execution", func(t *testing.T) {
		// B and C are in the same layer. Use a channel barrier to prove
		// both goroutines are running concurrently: each goroutine signals
		// arrival then waits for the other before completing.
		g := NewGraph()
		b, c := rid("svc", "b"), rid("svc", "c")
		g.AddNode(b)
		g.AddNode(c)

		plan := &OrderedPlan{
			Layers: [][]ResourceChange{
				{
					{ID: b, Type: ChangeCreate, Desired: dummyResource("svc", "b")},
					{ID: c, Type: ChangeCreate, Desired: dummyResource("svc", "c")},
				},
			},
		}

		// Both goroutines must reach the barrier for either to proceed.
		// If execution is sequential, this deadlocks — caught by the
		// local timeout below rather than the global test timeout.
		barrier := make(chan struct{}, 2)
		mock := &mockProvider{applyFn: func(_ context.Context, _ provider.Operation, _ provider.Resource) dcl.Diagnostics {
			barrier <- struct{}{} // signal arrival
			<-barrier             // wait for the other
			return nil
		}}

		done := make(chan *ApplyResult, 1)
		go func() {
			done <- Execute(context.Background(), plan, g, map[string]provider.Provider{"svc": mock})
		}()

		var result *ApplyResult
		select {
		case result = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("deadlock: layer resources appear to be running sequentially")
		}

		if len(result.Results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(result.Results))
		}
		for _, r := range result.Results {
			if r.Status != StatusSuccess {
				t.Errorf("resource %s: expected success, got %s", r.ID, r.Status)
			}
		}
	})
}
