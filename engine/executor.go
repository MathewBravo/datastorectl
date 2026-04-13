package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/MathewBravo/datastorectl/provider"
)

// Execute applies planned changes with per-layer concurrency.
// Resources within a layer run in parallel goroutines. Layer boundaries
// act as barriers — all resources in a layer complete before the next starts.
func Execute(ctx context.Context, plan *OrderedPlan, graph *Graph, providers map[string]provider.Provider) *ApplyResult {
	result := &ApplyResult{}
	skip := NewSkipTracker(graph)
	// mu guards skip and result.Results. All SkipTracker calls (ShouldSkip,
	// MarkFailed) and result mutations must be made under this lock.
	var mu sync.Mutex

	for _, layer := range plan.Layers {
		var wg sync.WaitGroup
		wg.Add(len(layer))

		for _, change := range layer {
			go func(change ResourceChange) {
				defer wg.Done()
				id := change.ID

				// No-ops succeed without calling the provider.
				if change.Type == ChangeNoOp {
					mu.Lock()
					result.Results = append(result.Results, ResourceResult{
						ID: id, Status: StatusSuccess, ChangeType: change.Type,
					})
					mu.Unlock()
					return
				}

				// Skip if a transitive dependency has failed.
				mu.Lock()
				shouldSkip := skip.ShouldSkip(id)
				mu.Unlock()
				if shouldSkip {
					mu.Lock()
					result.Results = append(result.Results, ResourceResult{
						ID: id, Status: StatusSkipped, ChangeType: change.Type,
					})
					mu.Unlock()
					return
				}

				// Respect context cancellation.
				if err := ctx.Err(); err != nil {
					mu.Lock()
					skip.MarkFailed(id)
					result.Results = append(result.Results, ResourceResult{
						ID: id, Status: StatusFailed, Error: err, ChangeType: change.Type,
					})
					mu.Unlock()
					return
				}

				// Look up the provider for this resource type.
				p, ok := providers[id.Type]
				if !ok {
					mu.Lock()
					skip.MarkFailed(id)
					result.Results = append(result.Results, ResourceResult{
						ID: id, Status: StatusFailed,
						Error:      fmt.Errorf("no provider registered for type %q", id.Type),
						ChangeType: change.Type,
					})
					mu.Unlock()
					return
				}

				// Determine operation and resource.
				op := changeToOp(change.Type)
				res := change.Desired
				if change.Type == ChangeDelete {
					res = change.Live
				}

				// Apply — no lock held during provider call.
				diags := p.Apply(ctx, op, *res)

				mu.Lock()
				if diags.HasErrors() {
					skip.MarkFailed(id)
					result.Results = append(result.Results, ResourceResult{
						ID: id, Status: StatusFailed,
						Error:      errors.New(diags.Error()),
						ChangeType: change.Type,
					})
				} else {
					result.Results = append(result.Results, ResourceResult{
						ID: id, Status: StatusSuccess, ChangeType: change.Type,
					})
				}
				mu.Unlock()
			}(change)
		}

		wg.Wait()
	}
	return result
}

func changeToOp(ct ChangeType) provider.Operation {
	switch ct {
	case ChangeCreate:
		return provider.OpCreate
	case ChangeUpdate:
		return provider.OpUpdate
	case ChangeDelete:
		return provider.OpDelete
	default:
		return provider.OpCreate // unreachable: no-ops handled before this call
	}
}
