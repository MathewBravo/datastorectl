package engine

import (
	"context"
	"fmt"

	"github.com/MathewBravo/datastorectl/dcl"
	"github.com/MathewBravo/datastorectl/provider"
)

// Engine is the top-level orchestrator that wires the pipeline steps into
// cohesive Plan, Apply, and DryRun operations.
type Engine struct {
	SecretResolver SecretResolver
}

// planResult bundles the outputs of the internal planning pipeline so that
// Apply and DryRun can reuse the providers map built during planning.
type planResult struct {
	plan      *Plan
	graph     *Graph
	providers map[string]provider.Provider
}

// plan runs the full planning pipeline: convert → configure → discover →
// build graph → resolve references → resolve secrets → normalize → build plan.
func (e *Engine) plan(ctx context.Context, file *dcl.File, configs map[string]*provider.OrderedMap) (*planResult, error) {
	// 1. Convert DCL file into a flat resource set.
	resourceSet, err := ConvertFile(file)
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
	}

	// 2. Look up, instantiate, and configure providers.
	providers, _, err := ConfigureProviders(ctx, resourceSet.Resources, configs)
	if err != nil {
		return nil, fmt.Errorf("configure providers: %w", err)
	}

	// 3. Discover live state from each unique provider.
	live, err := discover(ctx, providers)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}

	// 4. Build the dependency graph BEFORE resolution (needs KindReference values).
	graph, err := BuildDependencyGraph(resourceSet.Resources)
	if err != nil {
		return nil, fmt.Errorf("build dependency graph: %w", err)
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
			return nil, fmt.Errorf("resolve references: %s: %w", r.ID, err)
		}
		desired[i] = resolved
	}

	// 7. Resolve secret function calls in desired resources.
	for i, r := range desired {
		resolved, err := ResolveSecrets(ctx, r, e.SecretResolver)
		if err != nil {
			return nil, fmt.Errorf("resolve secrets: %s: %w", r.ID, err)
		}
		desired[i] = resolved
	}

	// 8. Normalize desired resources.
	normalizedDesired, err := NormalizeResources(ctx, desired, providers)
	if err != nil {
		return nil, fmt.Errorf("normalize desired: %w", err)
	}

	// 9. Normalize live resources.
	normalizedLive, err := NormalizeResources(ctx, live, providers)
	if err != nil {
		return nil, fmt.Errorf("normalize live: %w", err)
	}

	// 10. Build the plan by diffing desired against live.
	plan := BuildPlan(normalizedDesired, normalizedLive)

	// 11. Add live-only (delete) resources to the graph so OrderPlan includes them.
	for _, c := range plan.Changes {
		if c.Type == ChangeDelete && !graph.HasNode(c.ID) {
			graph.AddNode(c.ID)
		}
	}

	return &planResult{plan: plan, graph: graph, providers: providers}, nil
}

// Plan runs the full planning pipeline and returns the plan and dependency
// graph. It is a thin wrapper around the internal plan method.
func (e *Engine) Plan(ctx context.Context, file *dcl.File, configs map[string]*provider.OrderedMap) (*Plan, *Graph, error) {
	result, err := e.plan(ctx, file, configs)
	if err != nil {
		return nil, nil, err
	}
	return result.plan, result.graph, nil
}

// Apply runs the full pipeline: plan → validate → order → execute.
// The returned error covers pre-execution failures (plan, validate, cycle).
// Per-resource execution failures live in ApplyResult.Results.
func (e *Engine) Apply(ctx context.Context, file *dcl.File, configs map[string]*provider.OrderedMap) (*ApplyResult, error) {
	result, err := e.plan(ctx, file, configs)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}

	if err := validateResources(ctx, result.plan, result.providers); err != nil {
		return nil, err
	}

	orderedPlan, err := OrderPlan(result.plan, result.graph)
	if err != nil {
		return nil, fmt.Errorf("order plan: %w", err)
	}

	return Execute(ctx, orderedPlan, result.graph, result.providers), nil
}

// DryRun runs the planning pipeline with validation but does not order or
// execute. It catches configuration and validation problems without applying.
func (e *Engine) DryRun(ctx context.Context, file *dcl.File, configs map[string]*provider.OrderedMap) (*Plan, error) {
	result, err := e.plan(ctx, file, configs)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}

	if err := validateResources(ctx, result.plan, result.providers); err != nil {
		return nil, err
	}

	return result.plan, nil
}

// validateResources calls Validate on each create/update change in the plan.
// No-ops and deletes are skipped. It fails fast on the first validation error.
func validateResources(ctx context.Context, plan *Plan, providers map[string]provider.Provider) error {
	for _, change := range plan.Changes {
		if change.Type == ChangeNoOp || change.Type == ChangeDelete {
			continue
		}
		p, ok := providers[change.ID.Type]
		if !ok {
			return fmt.Errorf("resource %s: no provider for type %q", change.ID, change.ID.Type)
		}
		diags := p.Validate(ctx, *change.Desired)
		if diags.HasErrors() {
			return fmt.Errorf("resource %s: %s", change.ID, diags.Error())
		}
	}
	return nil
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
