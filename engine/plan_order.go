package engine

import "github.com/MathewBravo/datastorectl/provider"

// OrderedPlan holds resource changes grouped into dependency-aware layers.
type OrderedPlan struct {
	Layers [][]ResourceChange
}

// OrderPlan arranges a plan's changes into dependency-ordered layers.
// Creates and updates follow topological order (dependencies first).
// Deletes follow reverse topological order (dependents first).
func OrderPlan(plan *Plan, graph *Graph) (*OrderedPlan, error) {
	topoLayers, err := graph.TopologicalSort()
	if err != nil {
		return nil, err
	}

	// Index changes by ResourceID.
	changeIndex := make(map[provider.ResourceID]ResourceChange, len(plan.Changes))
	for _, c := range plan.Changes {
		changeIndex[c.ID] = c
	}

	var layers [][]ResourceChange

	// Forward pass: creates, updates, no-ops in topological order.
	for _, topoLayer := range topoLayers {
		var layer []ResourceChange
		for _, id := range topoLayer {
			c, ok := changeIndex[id]
			if !ok || c.Type == ChangeDelete {
				continue
			}
			layer = append(layer, c)
		}
		if len(layer) > 0 {
			layers = append(layers, layer)
		}
	}

	// Reverse pass: deletes in reverse topological order.
	for i := len(topoLayers) - 1; i >= 0; i-- {
		var layer []ResourceChange
		for _, id := range topoLayers[i] {
			c, ok := changeIndex[id]
			if !ok || c.Type != ChangeDelete {
				continue
			}
			layer = append(layer, c)
		}
		if len(layer) > 0 {
			layers = append(layers, layer)
		}
	}

	return &OrderedPlan{Layers: layers}, nil
}
