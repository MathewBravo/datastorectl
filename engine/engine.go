package engine

import (
	"context"
	"fmt"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// Engine is the top-level orchestrator that wires the pipeline steps into
// cohesive Plan and (future) Apply operations.
type Engine struct {
	SecretResolver SecretResolver
}

// Plan runs the full planning pipeline: convert → configure → discover →
// build graph → resolve references → resolve secrets → normalize → build plan.
// It returns the plan, the dependency graph (needed by callers for ordering
// and execution), and any error encountered along the way.
func (e *Engine) Plan(ctx context.Context, file *dcl.File, configs map[string]*provider.OrderedMap) (*Plan, *Graph, error) {
	// 1. Convert DCL file into a flat resource set.
	resourceSet, err := ConvertFile(file)
	if err != nil {
		return nil, nil, fmt.Errorf("convert: %w", err)
	}

	// 2. Look up, instantiate, and configure providers.
	providers, err := ConfigureProviders(ctx, resourceSet.Resources, configs)
	if err != nil {
		return nil, nil, fmt.Errorf("configure providers: %w", err)
	}

	// 3. Discover live state from each unique provider.
	live, err := discover(ctx, providers)
	if err != nil {
		return nil, nil, fmt.Errorf("discover: %w", err)
	}

	// 4. Build the dependency graph BEFORE resolution (needs KindReference values).
	graph, err := BuildDependencyGraph(resourceSet.Resources)
	if err != nil {
		return nil, nil, fmt.Errorf("build dependency graph: %w", err)
	}

	// 5. Build an index of desired resources for reference resolution.
	desired := resourceSet.Resources
	index := make(map[provider.ResourceID]provider.Resource, len(desired))
	for _, r := range desired {
		index[r.ID] = r
	}

	// 6. Resolve cross-resource references in desired resources.
	for i, r := range desired {
		resolved, err := ResolveReferences(r, index)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve references: %s: %w", r.ID, err)
		}
		desired[i] = resolved
	}

	// 7. Resolve secret function calls in desired resources.
	for i, r := range desired {
		resolved, err := ResolveSecrets(ctx, r, e.SecretResolver)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve secrets: %s: %w", r.ID, err)
		}
		desired[i] = resolved
	}

	// 8. Normalize desired resources.
	normalizedDesired, err := NormalizeResources(ctx, desired, providers)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize desired: %w", err)
	}

	// 9. Normalize live resources.
	normalizedLive, err := NormalizeResources(ctx, live, providers)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize live: %w", err)
	}

	// 10. Build the plan by diffing desired against live.
	plan := BuildPlan(normalizedDesired, normalizedLive)

	return plan, graph, nil
}

// discover calls Discover on each unique provider instance, deduplicating by
// pointer identity so providers shared across multiple resource types are only
// called once. It extends the providers map in-place for any live-only resource
// types so that NormalizeResources can find a provider for every live resource.
func discover(ctx context.Context, providers map[string]provider.Provider) ([]provider.Resource, error) {
	seen := make(map[string]provider.Provider) // pointer address → provider
	for _, p := range providers {
		addr := fmt.Sprintf("%p", p)
		seen[addr] = p
	}

	var live []provider.Resource
	for _, p := range seen {
		resources, diags := p.Discover(ctx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("%s", diags.Error())
		}
		for _, r := range resources {
			if _, ok := providers[r.ID.Type]; !ok {
				providers[r.ID.Type] = p
			}
		}
		live = append(live, resources...)
	}

	return live, nil
}
