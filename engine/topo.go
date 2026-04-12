package engine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/MathewBravo/datastorectl/provider"
)

// CycleError reports a dependency cycle found during topological sorting.
// Cycle contains the nodes forming the cycle, with the first node repeated
// at the end to close the loop (e.g. [A, B, C, A]).
type CycleError struct {
	Cycle []provider.ResourceID
}

func (e *CycleError) Error() string {
	parts := make([]string, len(e.Cycle))
	for i, id := range e.Cycle {
		parts[i] = id.String()
	}
	return fmt.Sprintf("dependency cycle: %s", strings.Join(parts, " -> "))
}

// TopologicalSort returns the nodes of the graph grouped into layers.
// Each layer contains nodes whose dependencies have all been satisfied by
// prior layers. Nodes within a layer are sorted by ResourceID.String() for
// determinism. If the graph contains a cycle, a *CycleError is returned.
func (g *Graph) TopologicalSort() ([][]provider.ResourceID, error) {
	nodes := g.Nodes()
	if len(nodes) == 0 {
		return nil, nil
	}

	// Build in-degree map.
	inDeg := make(map[provider.ResourceID]int, len(nodes))
	for _, n := range nodes {
		inDeg[n] = len(g.DependsOn(n))
	}

	// Collect layer 0: nodes with no dependencies.
	var layer []provider.ResourceID
	for _, n := range nodes {
		if inDeg[n] == 0 {
			layer = append(layer, n)
		}
	}
	sortByString(layer)

	var result [][]provider.ResourceID
	processed := 0

	for len(layer) > 0 {
		result = append(result, layer)
		processed += len(layer)

		var next []provider.ResourceID
		for _, n := range layer {
			for _, dep := range g.DependedOnBy(n) {
				inDeg[dep]--
				if inDeg[dep] == 0 {
					next = append(next, dep)
				}
			}
		}
		sortByString(next)
		layer = next
	}

	if processed < len(nodes) {
		remaining := make(map[provider.ResourceID]struct{})
		for _, n := range nodes {
			if inDeg[n] > 0 {
				remaining[n] = struct{}{}
			}
		}
		return nil, &CycleError{Cycle: findCycle(g, remaining)}
	}

	return result, nil
}

// findCycle extracts one cycle from a set of nodes known to be in a cycle.
// It walks forward edges deterministically and returns the cycle with the
// starting node repeated at the end.
func findCycle(g *Graph, remaining map[provider.ResourceID]struct{}) []provider.ResourceID {
	// Pick the lexicographically smallest starting node for determinism.
	starts := make([]provider.ResourceID, 0, len(remaining))
	for n := range remaining {
		starts = append(starts, n)
	}
	sortByString(starts)
	current := starts[0]

	// Walk forward edges, always choosing the smallest target in remaining.
	order := []provider.ResourceID{current}
	visited := map[provider.ResourceID]int{current: 0}

	for {
		deps := g.DependsOn(current)
		// Filter to edges within remaining, then sort.
		var candidates []provider.ResourceID
		for _, d := range deps {
			if _, ok := remaining[d]; ok {
				candidates = append(candidates, d)
			}
		}
		sortByString(candidates)
		next := candidates[0]

		if idx, seen := visited[next]; seen {
			// Extract the cycle from first occurrence to current, then close.
			cycle := make([]provider.ResourceID, len(order[idx:]))
			copy(cycle, order[idx:])
			cycle = append(cycle, next)
			return cycle
		}

		visited[next] = len(order)
		order = append(order, next)
		current = next
	}
}

func sortByString(s []provider.ResourceID) {
	slices.SortFunc(s, func(a, b provider.ResourceID) int {
		return strings.Compare(a.String(), b.String())
	})
}
