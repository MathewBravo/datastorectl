package engine

import (
	"context"
	"fmt"

	"github.com/MathewBravo/datastorectl/config"
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

// plan runs the full planning pipeline: split → convert → configure → discover →
// build graph → resolve references → resolve secrets → normalize → build plan.
func (e *Engine) plan(ctx context.Context, file *dcl.File, configs map[string]*provider.OrderedMap, opts PlanOptions) (*planResult, error) {
	if file == nil {
		return nil, fmt.Errorf("convert: cannot convert nil file")
	}
	if file.Diagnostics.HasErrors() {
		return nil, fmt.Errorf("convert: file has parse errors: %s", file.Diagnostics.Error())
	}

	// 1. Split file into context and resource blocks.
	contextBlocks, resourceBlocks := config.SplitFile(file)

	// 2. Collect schemas from providers for the resource types in this file.
	schemas := collectSchemas(resourceBlocks)

	// 3. Convert resource blocks (not context blocks) into a flat resource set.
	resourceSet, err := ConvertBlocks(resourceBlocks, schemas)
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
	}
	desired := resourceSet.Resources

	// 4. If file has context blocks, parse them and wire up configs.
	if len(contextBlocks) > 0 {
		contexts, err := config.ParseContexts(contextBlocks)
		if err != nil {
			return nil, fmt.Errorf("parse contexts: %w", err)
		}
		desired, err = config.ResolveResourceContexts(desired, contexts)
		if err != nil {
			return nil, fmt.Errorf("resolve resource contexts: %w", err)
		}
		if configs == nil {
			configs, err = config.BuildConfigs(contexts)
			if err != nil {
				return nil, fmt.Errorf("build configs: %w", err)
			}
			if err := config.ResolveConfigSecrets(ctx, configs, e.SecretResolver); err != nil {
				return nil, fmt.Errorf("resolve config secrets: %w", err)
			}
		}
	}

	// 5. Look up, instantiate, and configure providers.
	providers, orderings, err := ConfigureProviders(ctx, desired, configs)
	if err != nil {
		return nil, fmt.Errorf("configure providers: %w", err)
	}

	// 6. Discover live state.
	allLive, err := discover(ctx, providers)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}

	// 6b. Scope live resources to declared types.
	desiredTypes := make(map[string]struct{}, len(desired))
	for _, r := range desired {
		desiredTypes[r.ID.Type] = struct{}{}
	}
	var live []provider.Resource
	for _, r := range allLive {
		if _, ok := desiredTypes[r.ID.Type]; ok {
			live = append(live, r)
		}
	}

	// 7. Build dependency graph.
	graph, err := BuildDependencyGraphWithOrdering(desired, orderings)
	if err != nil {
		return nil, fmt.Errorf("build dependency graph: %w", err)
	}

	// 8. Build index for reference resolution.
	index := make(map[provider.ResourceID]provider.Resource, len(desired))
	for _, r := range desired {
		index[r.ID] = r
	}

	// 9. Resolve cross-resource references.
	for i, r := range desired {
		resolved, err := ResolveReferences(r, index)
		if err != nil {
			return nil, fmt.Errorf("resolve references: %s: %w", r.ID, err)
		}
		desired[i] = resolved
	}

	// 10. Resolve secrets.
	for i, r := range desired {
		resolved, err := ResolveSecrets(ctx, r, e.SecretResolver)
		if err != nil {
			return nil, fmt.Errorf("resolve secrets: %s: %w", r.ID, err)
		}
		desired[i] = resolved
	}

	// 11. Normalize desired.
	normalizedDesired, err := NormalizeResources(ctx, desired, providers)
	if err != nil {
		return nil, fmt.Errorf("normalize desired: %w", err)
	}

	// 12. Normalize live.
	normalizedLive, err := NormalizeResources(ctx, live, providers)
	if err != nil {
		return nil, fmt.Errorf("normalize live: %w", err)
	}

	// 13. Build full plan.
	plan := BuildPlan(normalizedDesired, normalizedLive)

	// 14. Apply prune policy: when Prune=false, split deletes out of Changes
	// into Unmanaged. The graph and executor only see Changes, so suppressing
	// here is sufficient to make the rest of the pipeline additive-only.
	if !opts.Prune {
		kept := make([]ResourceChange, 0, len(plan.Changes))
		var unmanaged []ResourceChange
		for _, c := range plan.Changes {
			if c.Type == ChangeDelete {
				unmanaged = append(unmanaged, c)
				continue
			}
			kept = append(kept, c)
		}
		plan.Changes = kept
		plan.Unmanaged = unmanaged
	}

	// 15. Add live-only delete nodes to the graph when they'll execute.
	for _, c := range plan.Changes {
		if c.Type == ChangeDelete && !graph.HasNode(c.ID) {
			graph.AddNode(c.ID)
		}
	}

	return &planResult{plan: plan, graph: graph, providers: providers}, nil
}

// Plan runs the full planning pipeline and returns the plan and dependency
// graph. Pass PlanOptions{Prune: true} to include deletes in Plan.Changes.
func (e *Engine) Plan(ctx context.Context, file *dcl.File, configs map[string]*provider.OrderedMap, opts PlanOptions) (*Plan, *Graph, error) {
	result, err := e.plan(ctx, file, configs, opts)
	if err != nil {
		return nil, nil, err
	}
	return result.plan, result.graph, nil
}

// Apply runs the full pipeline: plan → validate → order → execute.
// With PlanOptions{Prune: false} (default), deletes are skipped entirely.
func (e *Engine) Apply(ctx context.Context, file *dcl.File, configs map[string]*provider.OrderedMap, opts PlanOptions) (*ApplyResult, error) {
	result, err := e.plan(ctx, file, configs, opts)
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

// DryRun runs the planning pipeline with validation but does not execute.
func (e *Engine) DryRun(ctx context.Context, file *dcl.File, configs map[string]*provider.OrderedMap, opts PlanOptions) (*Plan, error) {
	result, err := e.plan(ctx, file, configs, opts)
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

// collectSchemas extracts provider prefixes from the resource blocks, looks up
// each registered provider, and collects their declared schemas. This runs
// before conversion so the converter can use schema hints for list vs map.
// If a provider is not registered or declares no schemas, it is silently
// skipped — errors are caught later during ConfigureProviders.
func collectSchemas(blocks []dcl.Block) map[string]provider.Schema {
	// Deduplicate provider prefixes.
	prefixes := make(map[string]struct{})
	for _, b := range blocks {
		if prefix, ok := provider.ProviderForResourceType(b.Type); ok {
			prefixes[prefix] = struct{}{}
		}
	}

	schemas := make(map[string]provider.Schema)
	for prefix := range prefixes {
		f, ok := provider.Lookup(prefix)
		if !ok {
			continue
		}
		p := f()
		for typ, s := range p.Schemas() {
			schemas[typ] = s
		}
	}

	if len(schemas) == 0 {
		return nil
	}
	return schemas
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
