package engine

import (
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// makeResource builds a provider.Resource whose body contains a RefVal for
// each supplied ref target. This keeps tests concise.
func makeResource(typ, name string, refs ...provider.ResourceID) provider.Resource {
	body := provider.NewOrderedMap()
	for i, ref := range refs {
		key := string(rune('a' + i))
		body.Set(key, provider.RefVal([]string{ref.Type, ref.Name}))
	}
	return provider.Resource{
		ID:   provider.ResourceID{Type: typ, Name: name},
		Body: body,
	}
}

func rid(typ, name string) provider.ResourceID {
	return provider.ResourceID{Type: typ, Name: name}
}

func TestBuildDependencyGraph(t *testing.T) {
	t.Run("empty_input", func(t *testing.T) {
		g, err := BuildDependencyGraph(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(g.Nodes()) != 0 {
			t.Fatalf("expected 0 nodes, got %d", len(g.Nodes()))
		}
	})

	t.Run("single_resource_no_refs", func(t *testing.T) {
		r := makeResource("db", "main")
		g, err := BuildDependencyGraph([]provider.Resource{r})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(g.Nodes()) != 1 {
			t.Fatalf("expected 1 node, got %d", len(g.Nodes()))
		}
		if len(g.DependsOn(r.ID)) != 0 {
			t.Fatalf("expected 0 edges, got %d", len(g.DependsOn(r.ID)))
		}
	})

	t.Run("single_reference", func(t *testing.T) {
		db := makeResource("db", "main")
		app := makeResource("app", "web", rid("db", "main"))
		g, err := BuildDependencyGraph([]provider.Resource{db, app})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(g.Nodes()) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(g.Nodes()))
		}
		deps := g.DependsOn(app.ID)
		if len(deps) != 1 {
			t.Fatalf("expected 1 dependency, got %d", len(deps))
		}
		if deps[0] != db.ID {
			t.Fatalf("expected dependency on %v, got %v", db.ID, deps[0])
		}
	})

	t.Run("multiple_references", func(t *testing.T) {
		db := makeResource("db", "main")
		cache := makeResource("cache", "redis")
		app := makeResource("app", "web", rid("db", "main"), rid("cache", "redis"))
		g, err := BuildDependencyGraph([]provider.Resource{db, cache, app})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(g.Nodes()) != 3 {
			t.Fatalf("expected 3 nodes, got %d", len(g.Nodes()))
		}
		deps := g.DependsOn(app.ID)
		if len(deps) != 2 {
			t.Fatalf("expected 2 dependencies, got %d", len(deps))
		}
	})

	t.Run("transitive_chain", func(t *testing.T) {
		c := makeResource("c", "svc")
		b := makeResource("b", "svc", rid("c", "svc"))
		a := makeResource("a", "svc", rid("b", "svc"))
		g, err := BuildDependencyGraph([]provider.Resource{a, b, c})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		aDeps := g.DependsOn(a.ID)
		if len(aDeps) != 1 || aDeps[0] != b.ID {
			t.Fatalf("expected a→b, got %v", aDeps)
		}
		bDeps := g.DependsOn(b.ID)
		if len(bDeps) != 1 || bDeps[0] != c.ID {
			t.Fatalf("expected b→c, got %v", bDeps)
		}
		cDeps := g.DependsOn(c.ID)
		if len(cDeps) != 0 {
			t.Fatalf("expected c to have no deps, got %v", cDeps)
		}
	})

	t.Run("duplicate_refs_deduped", func(t *testing.T) {
		db := makeResource("db", "main")
		// Build a resource with two refs to the same target manually.
		body := provider.NewOrderedMap()
		body.Set("a", provider.RefVal([]string{"db", "main"}))
		body.Set("b", provider.RefVal([]string{"db", "main"}))
		app := provider.Resource{
			ID:   rid("app", "web"),
			Body: body,
		}
		g, err := BuildDependencyGraph([]provider.Resource{db, app})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		deps := g.DependsOn(app.ID)
		if len(deps) != 1 {
			t.Fatalf("expected 1 edge (deduped), got %d", len(deps))
		}
	})

	t.Run("self_reference", func(t *testing.T) {
		body := provider.NewOrderedMap()
		body.Set("self", provider.RefVal([]string{"db", "main"}))
		r := provider.Resource{
			ID:   rid("db", "main"),
			Body: body,
		}
		g, err := BuildDependencyGraph([]provider.Resource{r})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		deps := g.DependsOn(r.ID)
		if len(deps) != 1 || deps[0] != r.ID {
			t.Fatalf("expected self-loop edge, got %v", deps)
		}
	})
}

func TestBuildDependencyGraphWithOrdering(t *testing.T) {
	t.Run("nil_orderings", func(t *testing.T) {
		db := makeResource("db", "main")
		app := makeResource("app", "web", rid("db", "main"))
		g, err := BuildDependencyGraphWithOrdering([]provider.Resource{db, app}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		deps := g.DependsOn(app.ID)
		if len(deps) != 1 || deps[0] != db.ID {
			t.Fatalf("expected app→db, got %v", deps)
		}
	})

	t.Run("single_ordering", func(t *testing.T) {
		idx := makeResource("index", "hot")
		ism := makeResource("ism_policy", "delete")
		orderings := []provider.TypeOrdering{{Before: "index", After: "ism_policy"}}
		g, err := BuildDependencyGraphWithOrdering([]provider.Resource{idx, ism}, orderings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		deps := g.DependsOn(ism.ID)
		if len(deps) != 1 || deps[0] != idx.ID {
			t.Fatalf("expected ism_policy→index, got %v", deps)
		}
	})

	t.Run("ordering_no_matching_types", func(t *testing.T) {
		db := makeResource("db", "main")
		orderings := []provider.TypeOrdering{{Before: "foo", After: "bar"}}
		g, err := BuildDependencyGraphWithOrdering([]provider.Resource{db}, orderings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(g.Nodes()) != 1 {
			t.Fatalf("expected 1 node, got %d", len(g.Nodes()))
		}
		if len(g.DependsOn(db.ID)) != 0 {
			t.Fatalf("expected 0 edges, got %d", len(g.DependsOn(db.ID)))
		}
	})

	t.Run("ordering_one_side_empty", func(t *testing.T) {
		idx := makeResource("index", "hot")
		orderings := []provider.TypeOrdering{{Before: "index", After: "ism_policy"}}
		g, err := BuildDependencyGraphWithOrdering([]provider.Resource{idx}, orderings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(g.DependsOn(idx.ID)) != 0 {
			t.Fatalf("expected 0 edges when After side is empty, got %d", len(g.DependsOn(idx.ID)))
		}
	})

	t.Run("ordering_plus_references", func(t *testing.T) {
		db := makeResource("db", "main")
		cache := makeResource("cache", "redis")
		app := makeResource("app", "web", rid("db", "main"))
		orderings := []provider.TypeOrdering{{Before: "db", After: "cache"}}
		g, err := BuildDependencyGraphWithOrdering([]provider.Resource{db, cache, app}, orderings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// app depends on db via reference.
		appDeps := g.DependsOn(app.ID)
		if len(appDeps) != 1 || appDeps[0] != db.ID {
			t.Fatalf("expected app→db ref edge, got %v", appDeps)
		}
		// cache depends on db via ordering.
		cacheDeps := g.DependsOn(cache.ID)
		if len(cacheDeps) != 1 || cacheDeps[0] != db.ID {
			t.Fatalf("expected cache→db ordering edge, got %v", cacheDeps)
		}
	})

	t.Run("multiple_orderings", func(t *testing.T) {
		a := makeResource("a", "svc")
		b := makeResource("b", "svc")
		c := makeResource("c", "svc")
		orderings := []provider.TypeOrdering{
			{Before: "a", After: "b"},
			{Before: "b", After: "c"},
		}
		g, err := BuildDependencyGraphWithOrdering([]provider.Resource{a, b, c}, orderings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		bDeps := g.DependsOn(b.ID)
		if len(bDeps) != 1 || bDeps[0] != a.ID {
			t.Fatalf("expected b→a, got %v", bDeps)
		}
		cDeps := g.DependsOn(c.ID)
		if len(cDeps) != 1 || cDeps[0] != b.ID {
			t.Fatalf("expected c→b, got %v", cDeps)
		}
	})

	t.Run("ordering_cross_product", func(t *testing.T) {
		idx1 := makeResource("index", "hot")
		idx2 := makeResource("index", "cold")
		ism1 := makeResource("ism_policy", "delete")
		ism2 := makeResource("ism_policy", "archive")
		orderings := []provider.TypeOrdering{{Before: "index", After: "ism_policy"}}
		g, err := BuildDependencyGraphWithOrdering(
			[]provider.Resource{idx1, idx2, ism1, ism2}, orderings,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Each ism_policy should depend on both indices = 4 ordering edges total.
		ism1Deps := g.DependsOn(ism1.ID)
		if len(ism1Deps) != 2 {
			t.Fatalf("expected ism1 to depend on 2 indices, got %d", len(ism1Deps))
		}
		ism2Deps := g.DependsOn(ism2.ID)
		if len(ism2Deps) != 2 {
			t.Fatalf("expected ism2 to depend on 2 indices, got %d", len(ism2Deps))
		}
	})
}

func TestBuildDependencyGraph_Errors(t *testing.T) {
	t.Run("unresolved_single", func(t *testing.T) {
		app := makeResource("app", "web", rid("db", "missing"))
		_, err := BuildDependencyGraph([]provider.Resource{app})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unresolved references") {
			t.Fatalf("expected 'unresolved references' in error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "db.missing") {
			t.Fatalf("expected 'db.missing' in error, got: %v", err)
		}
	})

	t.Run("unresolved_multiple", func(t *testing.T) {
		app := makeResource("app", "web", rid("db", "gone"), rid("cache", "missing"))
		_, err := BuildDependencyGraph([]provider.Resource{app})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unresolved references") {
			t.Fatalf("expected 'unresolved references' in error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "cache.missing") {
			t.Fatalf("expected 'cache.missing' in error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "db.gone") {
			t.Fatalf("expected 'db.gone' in error, got: %v", err)
		}
	})
}
