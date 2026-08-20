package types

import "encoding/json"

/*
MarshalJSON preserves the native graph payload and overlays derived SCM nodes,
causal links, and the complete reasoning projection for visual inspection.
*/
func (graph *Graph) MarshalJSON() ([]byte, error) {
	type graphAlias Graph
	encoded, err := json.Marshal((*graphAlias)(graph))

	if err != nil {
		return nil, err
	}

	payload := make(map[string]interface{})

	if err := json.Unmarshal(encoded, &payload); err != nil {
		return nil, err
	}

	topology := graph.ReasoningTopology()
	payload["reasoning"] = topology
	overlayReasoningNodes(payload, topology)
	overlayReasoningLinks(payload, topology)
	return json.Marshal(payload)
}

func overlayReasoningNodes(
	payload map[string]interface{},
	topology ReasoningTopology,
) {
	nodes, supported := payload["nodes"].(map[string]interface{})

	if !supported {
		return
	}

	for _, node := range topology.Nodes {
		if !node.Derived {
			continue
		}

		nodes[node.ID] = map[string]interface{}{
			"id":         node.ID,
			"label":      node.Label,
			"symbol":     node.Symbol,
			"source":     node.Source,
			"kind":       "causal",
			"value":      node.Value,
			"confidence": node.Confidence,
			"metadata": map[string]interface{}{
				"reasoningTier": node.Tier,
				"causalRole":    node.Role,
				"derived":       true,
			},
		}
	}
}

func overlayReasoningLinks(
	payload map[string]interface{},
	topology ReasoningTopology,
) {
	edges, supported := payload["edges"].([]interface{})

	if !supported {
		return
	}

	for _, link := range topology.Links {
		if !link.Derived {
			continue
		}

		edges = append(edges, map[string]interface{}{
			"from":       link.From,
			"to":         link.To,
			"relation":   link.Relation,
			"weight":     link.Weight,
			"confidence": link.Confidence,
			"derived":    true,
		})
	}

	payload["edges"] = edges
}
