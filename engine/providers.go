package engine

import (
	"context"
	"fmt"

	"github.com/MathewBravo/datastorectl/provider"
)

// ConfigureProviders looks up, instantiates, and configures providers for the
// given resources. Returns a map keyed by full resource type (e.g.,
// "opensearch_ism_policy") ready to pass to Execute and NormalizeResources.
// Each unique provider prefix is instantiated and configured once; multiple
// resource types sharing a prefix share the same provider instance.
func ConfigureProviders(ctx context.Context, resources []provider.Resource, configs map[string]*provider.OrderedMap) (map[string]provider.Provider, []provider.TypeOrdering, error) {
	// Collect unique prefixes and which resource types belong to each.
	prefixTypes := make(map[string][]string) // prefix → []resourceType
	for _, r := range resources {
		prefix, ok := provider.ProviderForResourceType(r.ID.Type)
		if !ok {
			return nil, nil, fmt.Errorf("resource type %q has no provider prefix", r.ID.Type)
		}
		if _, seen := prefixTypes[prefix]; !seen {
			prefixTypes[prefix] = nil
		}
		prefixTypes[prefix] = append(prefixTypes[prefix], r.ID.Type)
	}

	// Instantiate and configure each provider, build the return map.
	out := make(map[string]provider.Provider)
	var orderings []provider.TypeOrdering
	for prefix, types := range prefixTypes {
		factory, ok := provider.Lookup(prefix)
		if !ok {
			return nil, nil, fmt.Errorf("no provider registered for prefix %q (from resource type %q)", prefix, types[0])
		}

		p := factory()

		cfg := configs[prefix] // nil if not present — that's fine
		diags := p.Configure(ctx, cfg)
		if diags.HasErrors() {
			return nil, nil, fmt.Errorf("configuring provider %q: %s", prefix, diags.Error())
		}

		if to, ok := p.(provider.TypeOrderer); ok {
			orderings = append(orderings, to.TypeOrderings()...)
		}

		for _, typ := range types {
			out[typ] = p
		}
	}

	return out, orderings, nil
}
