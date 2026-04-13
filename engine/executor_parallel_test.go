package engine

import (
	"context"
	"testing"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

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
}
