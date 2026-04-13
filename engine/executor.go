package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/MathewBravo/datastorectl/provider"
)

// ExecuteSequential applies planned changes layer by layer in dependency order.
// It calls provider.Apply for each non-no-op change, propagating failures to
// skip dependents via the SkipTracker.
func ExecuteSequential(ctx context.Context, plan *OrderedPlan, graph *Graph, providers map[string]provider.Provider) *ApplyResult {
	result := &ApplyResult{}
	skip := NewSkipTracker(graph)

	for _, layer := range plan.Layers {
		for _, change := range layer {
			id := change.ID

			// No-ops succeed without calling the provider.
			if change.Type == ChangeNoOp {
				result.Results = append(result.Results, ResourceResult{
					ID: id, Status: StatusSuccess, ChangeType: change.Type,
				})
				continue
			}

			// Skip if a transitive dependency has failed.
			if skip.ShouldSkip(id) {
				result.Results = append(result.Results, ResourceResult{
					ID: id, Status: StatusSkipped, ChangeType: change.Type,
				})
				continue
			}

			// Respect context cancellation.
			if err := ctx.Err(); err != nil {
				skip.MarkFailed(id)
				result.Results = append(result.Results, ResourceResult{
					ID: id, Status: StatusFailed, Error: err, ChangeType: change.Type,
				})
				continue
			}

			// Look up the provider for this resource type.
			p, ok := providers[id.Type]
			if !ok {
				skip.MarkFailed(id)
				result.Results = append(result.Results, ResourceResult{
					ID: id, Status: StatusFailed,
					Error:      fmt.Errorf("no provider registered for type %q", id.Type),
					ChangeType: change.Type,
				})
				continue
			}

			// Determine operation and resource.
			op := changeToOp(change.Type)
			res := change.Desired
			if change.Type == ChangeDelete {
				res = change.Live
			}

			// Apply.
			diags := p.Apply(ctx, op, *res)
			if diags.HasErrors() {
				skip.MarkFailed(id)
				result.Results = append(result.Results, ResourceResult{
					ID: id, Status: StatusFailed,
					Error:      errors.New(diags.Error()),
					ChangeType: change.Type,
				})
				continue
			}

			result.Results = append(result.Results, ResourceResult{
				ID: id, Status: StatusSuccess, ChangeType: change.Type,
			})
		}
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
