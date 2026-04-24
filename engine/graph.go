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

// RelabelNode renames a node while preserving every edge that touches
// it. Used when a provider's Normalize rewrites ResourceID.Name so the
// plan's post-Normalize IDs stay wired to the graph built from
// reference-resolution-time IDs. No-op when old == new. No-op when
// old is not registered.
func (g *Graph) RelabelNode(old, new provider.ResourceID) {
	if old == new {
		return
	}
	if _, ok := g.nodes[old]; !ok {
		return
	}
	delete(g.nodes, old)
	g.nodes[new] = struct{}{}

	// Forward edges: old → *.
	deps := g.forward[old]
	delete(g.forward, old)
	g.forward[new] = deps
	// Update reverse pointers that referenced `old` as a dependent.
	for _, dep := range deps {
		for i, parent := range g.reverse[dep] {
			if parent == old {
				g.reverse[dep][i] = new
			}
		}
	}

	// Reverse edges: * → old.
	dependents := g.reverse[old]
	delete(g.reverse, old)
	g.reverse[new] = dependents
	for _, dep := range dependents {
		for i, child := range g.forward[dep] {
			if child == old {
				g.forward[dep][i] = new
			}
		}
	}
}
