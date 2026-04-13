package engine

import (
	"errors"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

func TestOrderPlan(t *testing.T) {
	t.Run("empty_plan", func(t *testing.T) {
		plan := &Plan{}
		graph := NewGraph()

		ordered, err := OrderPlan(plan, graph)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ordered.Layers) != 0 {
			t.Fatalf("expected 0 layers, got %d", len(ordered.Layers))
		}
	})

	t.Run("creates_follow_topo_order", func(t *testing.T) {
		// A depends on B, B depends on C → topo order: C, B, A
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")

		graph := NewGraph()
		graph.AddEdge(a, b)
		graph.AddEdge(b, c)

		plan := &Plan{Changes: []ResourceChange{
			{ID: a, Type: ChangeCreate},
			{ID: b, Type: ChangeCreate},
			{ID: c, Type: ChangeCreate},
		}}

		ordered, err := OrderPlan(plan, graph)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ordered.Layers) != 3 {
			t.Fatalf("expected 3 layers, got %d", len(ordered.Layers))
		}
		assertLayer(t, ordered.Layers[0], c)
		assertLayer(t, ordered.Layers[1], b)
		assertLayer(t, ordered.Layers[2], a)
	})

	t.Run("deletes_reverse_topo_order", func(t *testing.T) {
		// A depends on B, B depends on C → topo: C, B, A → reverse: A, B, C
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")

		graph := NewGraph()
		graph.AddEdge(a, b)
		graph.AddEdge(b, c)

		plan := &Plan{Changes: []ResourceChange{
			{ID: a, Type: ChangeDelete},
			{ID: b, Type: ChangeDelete},
			{ID: c, Type: ChangeDelete},
		}}

		ordered, err := OrderPlan(plan, graph)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ordered.Layers) != 3 {
			t.Fatalf("expected 3 layers, got %d", len(ordered.Layers))
		}
		assertLayer(t, ordered.Layers[0], a)
		assertLayer(t, ordered.Layers[1], b)
		assertLayer(t, ordered.Layers[2], c)
	})

	t.Run("mixed_creates_and_deletes", func(t *testing.T) {
		// A(create) depends on B(create); C(delete) depends on D(delete)
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")
		d := rid("r", "d")

		graph := NewGraph()
		graph.AddEdge(a, b)
		graph.AddEdge(c, d)

		plan := &Plan{Changes: []ResourceChange{
			{ID: a, Type: ChangeCreate},
			{ID: b, Type: ChangeCreate},
			{ID: c, Type: ChangeDelete},
			{ID: d, Type: ChangeDelete},
		}}

		ordered, err := OrderPlan(plan, graph)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Forward: topo is [b,d], [a,c] → creates only: [b], [a]
		// Reverse: reverse topo is [a,c], [b,d] → deletes only: [c], [d]
		// Total: [b], [a], [c], [d]
		if len(ordered.Layers) != 4 {
			t.Fatalf("expected 4 layers, got %d: %v", len(ordered.Layers), layerIDs(ordered))
		}
		assertLayer(t, ordered.Layers[0], b)
		assertLayer(t, ordered.Layers[1], a)
		assertLayer(t, ordered.Layers[2], c)
		assertLayer(t, ordered.Layers[3], d)
	})

	t.Run("no_ops_in_forward_layers", func(t *testing.T) {
		a := rid("r", "a")
		b := rid("r", "b")

		graph := NewGraph()
		graph.AddEdge(a, b)

		plan := &Plan{Changes: []ResourceChange{
			{ID: a, Type: ChangeNoOp},
			{ID: b, Type: ChangeNoOp},
		}}

		ordered, err := OrderPlan(plan, graph)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ordered.Layers) != 2 {
			t.Fatalf("expected 2 layers, got %d", len(ordered.Layers))
		}
		assertLayer(t, ordered.Layers[0], b)
		assertLayer(t, ordered.Layers[1], a)
	})

	t.Run("updates_in_forward_layers", func(t *testing.T) {
		a := rid("r", "a")
		b := rid("r", "b")

		graph := NewGraph()
		graph.AddEdge(a, b)

		plan := &Plan{Changes: []ResourceChange{
			{ID: a, Type: ChangeUpdate},
			{ID: b, Type: ChangeUpdate},
		}}

		ordered, err := OrderPlan(plan, graph)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ordered.Layers) != 2 {
			t.Fatalf("expected 2 layers, got %d", len(ordered.Layers))
		}
		assertLayer(t, ordered.Layers[0], b)
		assertLayer(t, ordered.Layers[1], a)
	})

	t.Run("cycle_returns_error", func(t *testing.T) {
		a := rid("r", "a")
		b := rid("r", "b")

		graph := NewGraph()
		graph.AddEdge(a, b)
		graph.AddEdge(b, a)

		plan := &Plan{Changes: []ResourceChange{
			{ID: a, Type: ChangeCreate},
			{ID: b, Type: ChangeCreate},
		}}

		_, err := OrderPlan(plan, graph)
		if err == nil {
			t.Fatal("expected error for cycle")
		}
		var ce *CycleError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *CycleError, got %T", err)
		}
	})

	t.Run("independent_resources_same_layer", func(t *testing.T) {
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")

		graph := NewGraph()
		graph.AddNode(a)
		graph.AddNode(b)
		graph.AddNode(c)

		plan := &Plan{Changes: []ResourceChange{
			{ID: a, Type: ChangeCreate},
			{ID: b, Type: ChangeCreate},
			{ID: c, Type: ChangeCreate},
		}}

		ordered, err := OrderPlan(plan, graph)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ordered.Layers) != 1 {
			t.Fatalf("expected 1 layer, got %d", len(ordered.Layers))
		}
		assertLayer(t, ordered.Layers[0], a, b, c)
	})

	t.Run("changes_not_in_graph_skipped", func(t *testing.T) {
		a := rid("r", "a")
		x := rid("r", "x") // not in graph

		graph := NewGraph()
		graph.AddNode(a)

		plan := &Plan{Changes: []ResourceChange{
			{ID: a, Type: ChangeCreate},
			{ID: x, Type: ChangeCreate},
		}}

		ordered, err := OrderPlan(plan, graph)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Only 'a' should appear; 'x' is not in the graph.
		total := 0
		for _, layer := range ordered.Layers {
			total += len(layer)
		}
		if total != 1 {
			t.Fatalf("expected 1 total change, got %d: %v", total, layerIDs(ordered))
		}
		assertLayer(t, ordered.Layers[0], a)
	})
}

// assertLayer checks that a layer contains exactly the expected resource IDs in order.
func assertLayer(t *testing.T, layer []ResourceChange, expected ...provider.ResourceID) {
	t.Helper()
	if len(layer) != len(expected) {
		t.Fatalf("layer: expected %d changes, got %d: %v", len(expected), len(layer), changeIDs(layer))
	}
	for i, want := range expected {
		if layer[i].ID != want {
			t.Errorf("layer[%d]: expected %v, got %v", i, want, layer[i].ID)
		}
	}
}

// changeIDs extracts ResourceIDs from a slice of ResourceChange for diagnostics.
func changeIDs(changes []ResourceChange) []provider.ResourceID {
	ids := make([]provider.ResourceID, len(changes))
	for i, c := range changes {
		ids[i] = c.ID
	}
	return ids
}

// layerIDs returns all layer IDs for diagnostics.
func layerIDs(op *OrderedPlan) [][]provider.ResourceID {
	out := make([][]provider.ResourceID, len(op.Layers))
	for i, layer := range op.Layers {
		out[i] = changeIDs(layer)
	}
	return out
}
