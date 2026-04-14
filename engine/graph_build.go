package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MathewBravo/datastorectl/provider"
)

// BuildDependencyGraph builds a dependency graph from cross-resource
// references. Returns an error if any reference targets a resource
// not in the input slice.
func BuildDependencyGraph(resources []provider.Resource) (*Graph, error) {
	return BuildDependencyGraphWithOrdering(resources, nil)
}

// BuildDependencyGraphWithOrdering builds a dependency graph from both
// cross-resource references and provider-declared type orderings.
// Returns an error if any reference targets a resource not in the
// input slice. Orderings mentioning types with no matching resources
// are silently ignored.
func BuildDependencyGraphWithOrdering(resources []provider.Resource, orderings []provider.TypeOrdering) (*Graph, error) {
	g := NewGraph()

	// Pass 1: register nodes, build indexes.
	index := make(map[provider.ResourceID]struct{}, len(resources))
	byType := make(map[string][]provider.ResourceID)
	for _, r := range resources {
		g.AddNode(r.ID)
		index[r.ID] = struct{}{}
		byType[r.ID.Type] = append(byType[r.ID.Type], r.ID)
	}

	// Pass 2: reference edges.
	missing := make(map[provider.ResourceID]struct{})
	for _, r := range resources {
		seen := make(map[provider.ResourceID]struct{})
		for _, ref := range ExtractReferences(r) {
			if _, ok := index[ref]; !ok {
				missing[ref] = struct{}{}
				continue
			}
			if _, dup := seen[ref]; dup {
				continue
			}
			seen[ref] = struct{}{}
			g.AddEdge(r.ID, ref)
		}
	}
	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for id := range missing {
			names = append(names, id.String())
		}
		sort.Strings(names)
		return nil, fmt.Errorf("unresolved references: %s", strings.Join(names, ", "))
	}

	// Pass 3: type-ordering edges.
	for _, o := range orderings {
		afterIDs := byType[o.After]
		beforeIDs := byType[o.Before]
		for _, a := range afterIDs {
			for _, b := range beforeIDs {
				g.AddEdge(a, b)
			}
		}
	}

	return g, nil
}
