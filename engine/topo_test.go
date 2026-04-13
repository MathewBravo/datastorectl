package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

func TestTopologicalSort_Basic(t *testing.T) {
	t.Run("empty_graph", func(t *testing.T) {
		g := NewGraph()
		layers, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if layers != nil {
			t.Fatalf("expected nil, got %v", layers)
		}
	})

	t.Run("single_node", func(t *testing.T) {
		g := NewGraph()
		a := rid("app", "web")
		g.AddNode(a)

		layers, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(layers) != 1 {
			t.Fatalf("expected 1 layer, got %d", len(layers))
		}
		if len(layers[0]) != 1 || layers[0][0] != a {
			t.Fatalf("expected [[%v]], got %v", a, layers)
		}
	})

	t.Run("two_independent_nodes", func(t *testing.T) {
		g := NewGraph()
		a := rid("app", "web")
		b := rid("db", "main")
		g.AddNode(a)
		g.AddNode(b)

		layers, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(layers) != 1 {
			t.Fatalf("expected 1 layer, got %d", len(layers))
		}
		if len(layers[0]) != 2 {
			t.Fatalf("expected 2 nodes in layer 0, got %d", len(layers[0]))
		}
		// Should be sorted: app.web < db.main
		if layers[0][0] != a || layers[0][1] != b {
			t.Fatalf("expected [%v, %v], got %v", a, b, layers[0])
		}
	})

	t.Run("simple_chain", func(t *testing.T) {
		g := NewGraph()
		a := rid("app", "a")
		b := rid("app", "b")
		c := rid("app", "c")
		// A depends on B, B depends on C → apply order: C, B, A
		g.AddEdge(a, b)
		g.AddEdge(b, c)

		layers, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(layers) != 3 {
			t.Fatalf("expected 3 layers, got %d", len(layers))
		}
		if layers[0][0] != c {
			t.Errorf("layer 0: expected %v, got %v", c, layers[0][0])
		}
		if layers[1][0] != b {
			t.Errorf("layer 1: expected %v, got %v", b, layers[1][0])
		}
		if layers[2][0] != a {
			t.Errorf("layer 2: expected %v, got %v", a, layers[2][0])
		}
	})

	t.Run("diamond", func(t *testing.T) {
		g := NewGraph()
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")
		d := rid("r", "d")
		// D→B, D→C, B→A, C→A
		g.AddEdge(d, b)
		g.AddEdge(d, c)
		g.AddEdge(b, a)
		g.AddEdge(c, a)

		layers, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(layers) != 3 {
			t.Fatalf("expected 3 layers, got %d", len(layers))
		}
		if len(layers[0]) != 1 || layers[0][0] != a {
			t.Errorf("layer 0: expected [%v], got %v", a, layers[0])
		}
		if len(layers[1]) != 2 || layers[1][0] != b || layers[1][1] != c {
			t.Errorf("layer 1: expected [%v, %v], got %v", b, c, layers[1])
		}
		if len(layers[2]) != 1 || layers[2][0] != d {
			t.Errorf("layer 2: expected [%v], got %v", d, layers[2])
		}
	})

	t.Run("wide_then_narrow", func(t *testing.T) {
		g := NewGraph()
		x := rid("r", "x")
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")
		// X→A, X→B, X→C
		g.AddEdge(x, a)
		g.AddEdge(x, b)
		g.AddEdge(x, c)

		layers, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(layers) != 2 {
			t.Fatalf("expected 2 layers, got %d", len(layers))
		}
		if len(layers[0]) != 3 {
			t.Fatalf("expected 3 nodes in layer 0, got %d", len(layers[0]))
		}
		if layers[0][0] != a || layers[0][1] != b || layers[0][2] != c {
			t.Errorf("layer 0: expected [a, b, c], got %v", layers[0])
		}
		if len(layers[1]) != 1 || layers[1][0] != x {
			t.Errorf("layer 1: expected [%v], got %v", x, layers[1])
		}
	})
}

func TestTopologicalSort_Layers(t *testing.T) {
	t.Run("parallel_chains", func(t *testing.T) {
		g := NewGraph()
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")
		d := rid("r", "d")
		// A→B, C→D (two independent chains)
		g.AddEdge(a, b)
		g.AddEdge(c, d)

		layers, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(layers) != 2 {
			t.Fatalf("expected 2 layers, got %d", len(layers))
		}
		if len(layers[0]) != 2 || layers[0][0] != b || layers[0][1] != d {
			t.Errorf("layer 0: expected [b, d], got %v", layers[0])
		}
		if len(layers[1]) != 2 || layers[1][0] != a || layers[1][1] != c {
			t.Errorf("layer 1: expected [a, c], got %v", layers[1])
		}
	})

	t.Run("complex_dag", func(t *testing.T) {
		g := NewGraph()
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")
		d := rid("r", "d")
		// A→C, B→C, D→A, D→B
		g.AddEdge(a, c)
		g.AddEdge(b, c)
		g.AddEdge(d, a)
		g.AddEdge(d, b)

		layers, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(layers) != 3 {
			t.Fatalf("expected 3 layers, got %d", len(layers))
		}
		if len(layers[0]) != 1 || layers[0][0] != c {
			t.Errorf("layer 0: expected [c], got %v", layers[0])
		}
		if len(layers[1]) != 2 || layers[1][0] != a || layers[1][1] != b {
			t.Errorf("layer 1: expected [a, b], got %v", layers[1])
		}
		if len(layers[2]) != 1 || layers[2][0] != d {
			t.Errorf("layer 2: expected [d], got %v", layers[2])
		}
	})

	t.Run("nodes_without_edges", func(t *testing.T) {
		g := NewGraph()
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")
		d := rid("r", "d")
		g.AddNode(a)
		g.AddNode(b)
		g.AddNode(c)
		// D→A (only edge)
		g.AddEdge(d, a)

		layers, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(layers) != 2 {
			t.Fatalf("expected 2 layers, got %d", len(layers))
		}
		// Layer 0: a, b, c (all have inDeg 0)
		if len(layers[0]) != 3 {
			t.Fatalf("expected 3 nodes in layer 0, got %d", len(layers[0]))
		}
		if layers[0][0] != a || layers[0][1] != b || layers[0][2] != c {
			t.Errorf("layer 0: expected [a, b, c], got %v", layers[0])
		}
		if len(layers[1]) != 1 || layers[1][0] != d {
			t.Errorf("layer 1: expected [d], got %v", d)
		}
	})
}

func TestTopologicalSort_CycleDetection(t *testing.T) {
	t.Run("self_loop", func(t *testing.T) {
		g := NewGraph()
		a := rid("r", "a")
		g.AddEdge(a, a)

		_, err := g.TopologicalSort()
		if err == nil {
			t.Fatal("expected error for self-loop")
		}
		var ce *CycleError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *CycleError, got %T", err)
		}
	})

	t.Run("two_node_cycle", func(t *testing.T) {
		g := NewGraph()
		a := rid("r", "a")
		b := rid("r", "b")
		g.AddEdge(a, b)
		g.AddEdge(b, a)

		_, err := g.TopologicalSort()
		if err == nil {
			t.Fatal("expected error for cycle")
		}
		var ce *CycleError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *CycleError, got %T", err)
		}
		if len(ce.Cycle) < 3 {
			t.Fatalf("expected cycle length >= 3 (includes closing node), got %d", len(ce.Cycle))
		}
		// First and last should be the same node.
		if ce.Cycle[0] != ce.Cycle[len(ce.Cycle)-1] {
			t.Errorf("cycle should close: first=%v last=%v", ce.Cycle[0], ce.Cycle[len(ce.Cycle)-1])
		}
	})

	t.Run("three_node_cycle", func(t *testing.T) {
		g := NewGraph()
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")
		g.AddEdge(a, b)
		g.AddEdge(b, c)
		g.AddEdge(c, a)

		_, err := g.TopologicalSort()
		if err == nil {
			t.Fatal("expected error for cycle")
		}
		var ce *CycleError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *CycleError, got %T", err)
		}
		// 3 unique nodes + closing = 4
		if len(ce.Cycle) != 4 {
			t.Fatalf("expected cycle length 4, got %d: %v", len(ce.Cycle), ce.Cycle)
		}
		if ce.Cycle[0] != ce.Cycle[len(ce.Cycle)-1] {
			t.Errorf("cycle should close: first=%v last=%v", ce.Cycle[0], ce.Cycle[len(ce.Cycle)-1])
		}
	})

	t.Run("cycle_with_tail", func(t *testing.T) {
		g := NewGraph()
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")
		d := rid("r", "d")
		// D→A, A→B, B→C, C→A (cycle among A, B, C; D is a tail)
		g.AddEdge(d, a)
		g.AddEdge(a, b)
		g.AddEdge(b, c)
		g.AddEdge(c, a)

		_, err := g.TopologicalSort()
		if err == nil {
			t.Fatal("expected error for cycle")
		}
		var ce *CycleError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *CycleError, got %T", err)
		}
		// Cycle should only contain A, B, C (not D).
		cycleNodes := make(map[provider.ResourceID]bool)
		for _, n := range ce.Cycle {
			cycleNodes[n] = true
		}
		if cycleNodes[d] {
			t.Errorf("cycle should not contain tail node %v: %v", d, ce.Cycle)
		}
	})
}

func TestTopologicalSort_Determinism(t *testing.T) {
	t.Run("sorted_within_layers", func(t *testing.T) {
		g := NewGraph()
		// Add nodes in reverse order to exercise sorting.
		z := rid("r", "z")
		m := rid("r", "m")
		a := rid("r", "a")
		g.AddNode(z)
		g.AddNode(m)
		g.AddNode(a)

		layers, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(layers) != 1 {
			t.Fatalf("expected 1 layer, got %d", len(layers))
		}
		if layers[0][0] != a || layers[0][1] != m || layers[0][2] != z {
			t.Errorf("expected sorted [a, m, z], got %v", layers[0])
		}
	})

	t.Run("repeated_calls_same_result", func(t *testing.T) {
		g := NewGraph()
		a := rid("r", "a")
		b := rid("r", "b")
		c := rid("r", "c")
		d := rid("r", "d")
		g.AddEdge(d, b)
		g.AddEdge(d, c)
		g.AddEdge(b, a)
		g.AddEdge(c, a)

		first, err := g.TopologicalSort()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for i := 0; i < 10; i++ {
			again, err := g.TopologicalSort()
			if err != nil {
				t.Fatalf("unexpected error on run %d: %v", i, err)
			}
			if len(again) != len(first) {
				t.Fatalf("run %d: layer count %d != %d", i, len(again), len(first))
			}
			for j := range first {
				if len(again[j]) != len(first[j]) {
					t.Fatalf("run %d layer %d: length %d != %d", i, j, len(again[j]), len(first[j]))
				}
				for k := range first[j] {
					if again[j][k] != first[j][k] {
						t.Errorf("run %d layer %d pos %d: %v != %v", i, j, k, again[j][k], first[j][k])
					}
				}
			}
		}
	})
}

func TestCycleError(t *testing.T) {
	t.Run("error_message_format", func(t *testing.T) {
		ce := &CycleError{
			Cycle: []provider.ResourceID{
				rid("db", "main"),
				rid("app", "web"),
				rid("db", "main"),
			},
		}
		want := "dependency cycle: db.main -> app.web -> db.main"
		got := ce.Error()
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("implements_error_interface", func(t *testing.T) {
		var err error = &CycleError{
			Cycle: []provider.ResourceID{rid("r", "a"), rid("r", "a")},
		}
		if err.Error() == "" {
			t.Fatal("expected non-empty error string")
		}
		var ce *CycleError
		if !errors.As(err, &ce) {
			t.Fatal("expected errors.As to succeed")
		}
	})
}

func TestFindCycle_Determinism(t *testing.T) {
	// Verify findCycle picks the lexicographically smallest starting node
	// and follows the smallest edges.
	g := NewGraph()
	a := rid("r", "a")
	b := rid("r", "b")
	c := rid("r", "c")
	g.AddEdge(a, b)
	g.AddEdge(b, c)
	g.AddEdge(c, a)

	remaining := map[provider.ResourceID]struct{}{
		a: {}, b: {}, c: {},
	}

	for i := 0; i < 10; i++ {
		cycle := findCycle(g, remaining)
		if len(cycle) != 4 {
			t.Fatalf("run %d: expected length 4, got %d", i, len(cycle))
		}
		want := fmt.Sprintf("[r.a r.b r.c r.a]")
		got := fmt.Sprintf("%v", cycle)
		if got != want {
			t.Errorf("run %d: got %s, want %s", i, got, want)
		}
	}
}
