package engine

import (
	"context"
	"fmt"

	"github.com/MathewBravo/datastorectl/provider"
)

// NormalizeResources runs provider.Normalize on each resource so the differ
// sees canonicalized values. Returns a new slice; the input is not mutated.
// Fails fast on the first missing provider or normalization error.
func NormalizeResources(ctx context.Context, resources []provider.Resource, providers map[string]provider.Provider) ([]provider.Resource, error) {
	out := make([]provider.Resource, len(resources))
	for i, r := range resources {
		p, ok := providers[r.ID.Type]
		if !ok {
			return nil, fmt.Errorf("no provider registered for type %q", r.ID.Type)
		}

		normalized, diags := p.Normalize(ctx, r)
		if diags.HasErrors() {
			return nil, fmt.Errorf("normalizing %s: %s", r.ID, diags.Error())
		}

		out[i] = normalized
	}
	return out, nil
}
