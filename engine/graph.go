package engine

import "github.com/MathewBravo/datastorectl/provider"

// Graph is a directed graph of resource dependencies.
// Forward edges record "from depends on to"; reverse edges record the inverse.
type Graph struct {
	nodes   map[provider.ResourceID]struct{}
	forward map[provider.ResourceID][]provider.ResourceID // node → its dependencies
	reverse map[provider.ResourceID][]provider.ResourceID // node → its dependents
}

// NewGraph returns an empty Graph ready for use.
func NewGraph() *Graph {
	return &Graph{
		nodes:   make(map[provider.ResourceID]struct{}),
		forward: make(map[provider.ResourceID][]provider.ResourceID),
		reverse: make(map[provider.ResourceID][]provider.ResourceID),
	}
}

// AddNode registers a node. It is a no-op if the node already exists.
func (g *Graph) AddNode(id provider.ResourceID) {
	g.nodes[id] = struct{}{}
}

// AddEdge adds a directed edge meaning "from depends on to".
// Both nodes are auto-registered if not already present.
func (g *Graph) AddEdge(from, to provider.ResourceID) {
	g.AddNode(from)
	g.AddNode(to)
	g.forward[from] = append(g.forward[from], to)
	g.reverse[to] = append(g.reverse[to], from)
}

// Nodes returns all registered nodes. Order is non-deterministic.
func (g *Graph) Nodes() []provider.ResourceID {
	out := make([]provider.ResourceID, 0, len(g.nodes))
	for id := range g.nodes {
		out = append(out, id)
	}
	return out
}

// HasNode reports whether id has been registered.
func (g *Graph) HasNode(id provider.ResourceID) bool {
	_, ok := g.nodes[id]
	return ok
}

// DependsOn returns the resources that id depends on (its prerequisites).
func (g *Graph) DependsOn(id provider.ResourceID) []provider.ResourceID {
	return g.forward[id]
}

// DependedOnBy returns the resources that depend on id (its dependents).
func (g *Graph) DependedOnBy(id provider.ResourceID) []provider.ResourceID {
	return g.reverse[id]
}
