package engine

import (
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

func TestGraph_Empty(t *testing.T) {
	g := NewGraph()
	if len(g.Nodes()) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(g.Nodes()))
	}
	id := provider.ResourceID{Type: "any", Name: "thing"}
	if g.HasNode(id) {
		t.Fatal("expected HasNode false on empty graph")
	}
}

func TestGraph_AddNode(t *testing.T) {
	t.Run("single_node", func(t *testing.T) {
		g := NewGraph()
		id := provider.ResourceID{Type: "db", Name: "main"}
		g.AddNode(id)

		if !g.HasNode(id) {
			t.Fatal("expected HasNode true")
		}
		if len(g.Nodes()) != 1 {
			t.Fatalf("expected 1 node, got %d", len(g.Nodes()))
		}
	})

	t.Run("duplicate_node", func(t *testing.T) {
		g := NewGraph()
		id := provider.ResourceID{Type: "db", Name: "main"}
		g.AddNode(id)
		g.AddNode(id)

		if len(g.Nodes()) != 1 {
			t.Fatalf("expected 1 node after duplicate add, got %d", len(g.Nodes()))
		}
	})

	t.Run("multiple_nodes", func(t *testing.T) {
		g := NewGraph()
		a := provider.ResourceID{Type: "db", Name: "main"}
		b := provider.ResourceID{Type: "cache", Name: "redis"}
		c := provider.ResourceID{Type: "app", Name: "web"}
		g.AddNode(a)
		g.AddNode(b)
		g.AddNode(c)

		if len(g.Nodes()) != 3 {
			t.Fatalf("expected 3 nodes, got %d", len(g.Nodes()))
		}
	})
}

func TestGraph_AddEdge(t *testing.T) {
	t.Run("creates_forward_edge", func(t *testing.T) {
		g := NewGraph()
		from := provider.ResourceID{Type: "app", Name: "web"}
		to := provider.ResourceID{Type: "db", Name: "main"}
		g.AddEdge(from, to)

		deps := g.DependsOn(from)
		if len(deps) != 1 {
			t.Fatalf("expected 1 dependency, got %d", len(deps))
		}
		if deps[0] != to {
			t.Fatalf("expected dependency %v, got %v", to, deps[0])
		}
	})

	t.Run("creates_reverse_edge", func(t *testing.T) {
		g := NewGraph()
		from := provider.ResourceID{Type: "app", Name: "web"}
		to := provider.ResourceID{Type: "db", Name: "main"}
		g.AddEdge(from, to)

		dependents := g.DependedOnBy(to)
		if len(dependents) != 1 {
			t.Fatalf("expected 1 dependent, got %d", len(dependents))
		}
		if dependents[0] != from {
			t.Fatalf("expected dependent %v, got %v", from, dependents[0])
		}
	})

	t.Run("auto_registers_nodes", func(t *testing.T) {
		g := NewGraph()
		from := provider.ResourceID{Type: "app", Name: "web"}
		to := provider.ResourceID{Type: "db", Name: "main"}
		g.AddEdge(from, to)

		if !g.HasNode(from) {
			t.Fatal("expected from node to be registered")
		}
		if !g.HasNode(to) {
			t.Fatal("expected to node to be registered")
		}
		if len(g.Nodes()) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(g.Nodes()))
		}
	})

	t.Run("multiple_edges_from_same_node", func(t *testing.T) {
		g := NewGraph()
		a := provider.ResourceID{Type: "app", Name: "web"}
		b := provider.ResourceID{Type: "db", Name: "main"}
		c := provider.ResourceID{Type: "cache", Name: "redis"}
		g.AddEdge(a, b)
		g.AddEdge(a, c)

		deps := g.DependsOn(a)
		if len(deps) != 2 {
			t.Fatalf("expected 2 dependencies, got %d", len(deps))
		}
		if deps[0] != b {
			t.Errorf("expected deps[0] = %v, got %v", b, deps[0])
		}
		if deps[1] != c {
			t.Errorf("expected deps[1] = %v, got %v", c, deps[1])
		}
	})
}

func TestGraph_HasNode(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		g := NewGraph()
		id := provider.ResourceID{Type: "db", Name: "main"}
		g.AddNode(id)

		if !g.HasNode(id) {
			t.Fatal("expected true")
		}
	})

	t.Run("missing", func(t *testing.T) {
		g := NewGraph()
		id := provider.ResourceID{Type: "db", Name: "main"}

		if g.HasNode(id) {
			t.Fatal("expected false")
		}
	})
}

func TestGraph_DependsOn(t *testing.T) {
	t.Run("no_dependencies", func(t *testing.T) {
		g := NewGraph()
		id := provider.ResourceID{Type: "db", Name: "main"}
		g.AddNode(id)

		deps := g.DependsOn(id)
		if len(deps) != 0 {
			t.Fatalf("expected 0 dependencies, got %d", len(deps))
		}
	})

	t.Run("returns_dependencies", func(t *testing.T) {
		g := NewGraph()
		app := provider.ResourceID{Type: "app", Name: "web"}
		db := provider.ResourceID{Type: "db", Name: "main"}
		cache := provider.ResourceID{Type: "cache", Name: "redis"}
		g.AddEdge(app, db)
		g.AddEdge(app, cache)

		deps := g.DependsOn(app)
		if len(deps) != 2 {
			t.Fatalf("expected 2 dependencies, got %d", len(deps))
		}
	})
}

func TestGraph_DependedOnBy(t *testing.T) {
	t.Run("no_dependents", func(t *testing.T) {
		g := NewGraph()
		id := provider.ResourceID{Type: "app", Name: "web"}
		g.AddNode(id)

		dependents := g.DependedOnBy(id)
		if len(dependents) != 0 {
			t.Fatalf("expected 0 dependents, got %d", len(dependents))
		}
	})

	t.Run("returns_dependents", func(t *testing.T) {
		g := NewGraph()
		db := provider.ResourceID{Type: "db", Name: "main"}
		app := provider.ResourceID{Type: "app", Name: "web"}
		worker := provider.ResourceID{Type: "worker", Name: "bg"}
		g.AddEdge(app, db)
		g.AddEdge(worker, db)

		dependents := g.DependedOnBy(db)
		if len(dependents) != 2 {
			t.Fatalf("expected 2 dependents, got %d", len(dependents))
		}
	})
}
