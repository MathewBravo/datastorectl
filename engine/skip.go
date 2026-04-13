package engine

import "github.com/MathewBravo/datastorectl/provider"

// SkipTracker determines whether a resource should be skipped because one of
// its transitive dependencies has failed. It wraps an immutable Graph and a
// set of failed resource IDs.
type SkipTracker struct {
	graph  *Graph
	failed map[provider.ResourceID]struct{}
}

// NewSkipTracker returns a SkipTracker backed by the given dependency graph.
// The graph is shared, not copied — it must not be mutated after construction.
func NewSkipTracker(g *Graph) *SkipTracker {
	return &SkipTracker{graph: g, failed: make(map[provider.ResourceID]struct{})}
}

// MarkFailed records id as a failed resource. Idempotent.
func (s *SkipTracker) MarkFailed(id provider.ResourceID) {
	s.failed[id] = struct{}{}
}

// ShouldSkip reports whether id has any transitive dependency that has been
// marked as failed. A resource that is itself failed but has no failed
// dependencies returns false — the caller distinguishes failed vs skipped.
func (s *SkipTracker) ShouldSkip(id provider.ResourceID) bool {
	queue := s.graph.DependsOn(id)
	visited := make(map[provider.ResourceID]struct{}, len(queue))
	for _, dep := range queue {
		visited[dep] = struct{}{}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, failed := s.failed[current]; failed {
			return true
		}
		for _, dep := range s.graph.DependsOn(current) {
			if _, seen := visited[dep]; !seen {
				visited[dep] = struct{}{}
				queue = append(queue, dep)
			}
		}
	}
	return false
}
