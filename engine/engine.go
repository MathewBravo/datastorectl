package engine

import (
	"context"
	"fmt"
	"strings"

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

	// 6b. Scope live resources to declared types. This prevents the engine
	// from planning deletes for resource types the user didn't declare
	// (e.g., built-in OpenSearch users).
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

	// 12a. Cross-resource validation. Providers that implement
	// CrossResourceValidator see the normalized desired set scoped to
	// the resource types they own. This catches classes of
	// misconfiguration that per-resource Validate cannot express —
	// e.g. two mysql_user blocks whose Normalize collapses them to the
	// same (user, host) identity, or a mysql_user/mysql_role pair that
	// map to the same MySQL 8 server row. Error diagnostics abort the
	// pipeline before diff.
	if err := validateCrossResources(ctx, normalizedDesired, providers); err != nil {
		return nil, err
	}

	// 12b. Relabel graph nodes whose Normalize changed ResourceID.Name.
	// mysql_user and similar providers encode a multi-dimensional
	// identity tuple (e.g. "user@host") into ID.Name during Normalize.
	// Without this relabel, OrderPlan correlates graph layers against
	// plan.Changes using mismatched IDs and silently drops resources
	// whose Normalize changed their ID. We preserve reference and
	// type-ordering edges by relabeling in place rather than rebuilding
	// (reference edges were computed from pre-resolution RefVals that
	// are no longer present on post-resolution resources).
	if len(desired) == len(normalizedDesired) {
		for i := range desired {
			if desired[i].ID != normalizedDesired[i].ID {
				graph.RelabelNode(desired[i].ID, normalizedDesired[i].ID)
			}
		}
	}

	// 13. Build full plan.
	plan := BuildPlan(ctx, normalizedDesired, normalizedLive, providers)

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

	// 15. Add live-only delete nodes to the graph for any deletes that remain
	// in Changes (only populated when Prune=true). When Prune=false, this
	// loop is a no-op because all deletes moved to Unmanaged in step 14.
	for _, c := range plan.Changes {
		if c.Type == ChangeDelete && !graph.HasNode(c.ID) {
			graph.AddNode(c.ID)
		}
	}

	// 16. Collect DeleteGuards from any provider that implements DeleteGuarder.
	plan.Guards = collectDeleteGuards(ctx, plan, providers)

	return &planResult{plan: plan, graph: graph, providers: providers}, nil
}

// collectDeleteGuards groups deletes by provider and asks each provider
// that implements DeleteGuarder to annotate them. Guards from all
// providers are concatenated. Guard diagnostic errors are ignored —
// a provider failing to produce guards should not block planning,
// but means we lose the apply-time safety check for that provider.
func collectDeleteGuards(ctx context.Context, plan *Plan, providers map[string]provider.Provider) []Guard {
	byProvider := make(map[provider.Provider][]provider.Resource)
	for _, c := range plan.Changes {
		if c.Type != ChangeDelete || c.Live == nil {
			continue
		}
		p, ok := providers[c.ID.Type]
		if !ok {
			continue
		}
		byProvider[p] = append(byProvider[p], *c.Live)
	}

	var guards []Guard
	for p, deletes := range byProvider {
		g, ok := p.(provider.DeleteGuarder)
		if !ok {
			continue
		}
		providerGuards, diags := g.GuardDeletes(ctx, deletes)
		if diags.HasErrors() {
			continue
		}
		for _, pg := range providerGuards {
			guards = append(guards, Guard{Resource: pg.Resource, Reason: pg.Reason})
		}
	}
	return guards
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
// If Plan.Guards is non-empty and opts.AllowSelfLockout is false, Apply
// refuses to execute any operation — guards are a hard stop.
func (e *Engine) Apply(ctx context.Context, file *dcl.File, configs map[string]*provider.OrderedMap, opts PlanOptions) (*ApplyResult, error) {
	result, err := e.plan(ctx, file, configs, opts)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}
	if len(result.plan.Guards) > 0 && !opts.AllowSelfLockout {
		lines := make([]string, len(result.plan.Guards))
		for i, g := range result.plan.Guards {
			lines[i] = fmt.Sprintf("  - %s: %s", g.Resource, g.Reason)
		}
		return nil, fmt.Errorf(
			"refusing to apply: %d delete(s) would lock out the authenticated caller:\n%s\n\npass --allow-self-lockout to override",
			len(result.plan.Guards),
			strings.Join(lines, "\n"),
		)
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

// validateCrossResources groups the normalized desired resources by their
// owning provider and invokes CrossResourceValidator on each provider that
// implements it. Resources whose type has no registered provider (should
// not happen after ConfigureProviders) are skipped silently.
//
// Returns a formatted error when any provider returns error-severity
// diagnostics. Warning-only diagnostics are dropped — the current pipeline
// has no surface for plan-time warnings; adding one is a separate change.
func validateCrossResources(ctx context.Context, resources []provider.Resource, providers map[string]provider.Provider) error {
	byProvider := make(map[provider.Provider][]provider.Resource)
	order := make([]provider.Provider, 0)
	for _, r := range resources {
		p, ok := providers[r.ID.Type]
		if !ok {
			continue
		}
		if _, seen := byProvider[p]; !seen {
			order = append(order, p)
		}
		byProvider[p] = append(byProvider[p], r)
	}

	var allDiags dcl.Diagnostics
	for _, p := range order {
		v, ok := p.(provider.CrossResourceValidator)
		if !ok {
			continue
		}
		diags := v.ValidateResources(ctx, byProvider[p])
		allDiags = append(allDiags, diags...)
	}
	if allDiags.HasErrors() {
		return fmt.Errorf("cross-resource validation: %s", allDiags.Error())
	}
	return nil
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
