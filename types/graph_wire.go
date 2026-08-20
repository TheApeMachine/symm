package types

import (
	"fmt"
	"sort"

	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func (graph *Graph) Wire() *wire.GraphFrameT {
	if graph == nil {
		return nil
	}

	return graph.Clone().wireSnapshot()
}

func (graph *Graph) wireSnapshot() *wire.GraphFrameT {

	nodeIDs := make([]string, 0, len(graph.Nodes))

	for id := range graph.Nodes {
		nodeIDs = append(nodeIDs, id)
	}

	sort.Strings(nodeIDs)
	nodes := make([]*wire.GraphNodeT, 0, len(nodeIDs))

	for _, id := range nodeIDs {
		nodes = append(nodes, graphNodeWire(graph.Nodes[id]))
	}

	edges := make([]*wire.GraphEdgeT, 0, len(graph.Edges))

	for _, edge := range graph.Edges {
		edges = append(edges, graphEdgeWire(edge))
	}

	topology := graph.ReasoningTopology()

	for _, node := range topology.Nodes {
		if !node.Derived {
			continue
		}

		nodes = append(nodes, &wire.GraphNodeT{
			Id: node.ID, Label: node.Label, Symbol: node.Symbol,
			Source: node.Source, Kind: string(KindCausal), Value: node.Value,
			Confidence: node.Confidence, Derived: true,
			Metadata: &wire.GraphMetadataT{
				Strings: []*wire.NamedStringT{
					{Name: "reasoningTier", Value: string(node.Tier)},
					{Name: "causalRole", Value: node.Role},
				},
				Bools: []*wire.NamedBoolT{{Name: "derived", Value: true}},
			},
		})
	}

	for _, link := range topology.Links {
		if !link.Derived {
			continue
		}

		edges = append(edges, &wire.GraphEdgeT{
			From: link.From, To: link.To, Relation: link.Relation,
			Weight: link.Weight, Confidence: link.Confidence, Derived: true,
		})
	}

	return &wire.GraphFrameT{
		At: timeNano(graph.At), ForecastHorizon: int64(graph.ForecastHorizon),
		TaskSkill: graph.TaskSkill, TaskSkillReady: graph.TaskSkillReady,
		DecisionTarget: graph.DecisionTarget, Nodes: nodes, Edges: edges,
		Reasoning: reasoningWire(topology),
	}
}

func graphNodeWire(node *Node) *wire.GraphNodeT {
	if node == nil {
		return nil
	}

	result := &wire.GraphNodeT{
		Id: node.ID, Symbol: node.Symbol, Peer: node.Peer, Source: node.Source,
		MeasurementId: node.MeasurementID, Metric: string(node.Metric),
		Side: string(node.Side), Kind: string(node.Kind), Value: node.Value,
		Strength: node.Strength, Confidence: node.Confidence, Maturity: node.Maturity,
		Unit: string(node.Unit), ObservedFrom: timeNano(node.ObservedFrom),
		Horizon: int64(node.Horizon), At: timeNano(node.At), Metadata: metadataWire(node.Metadata),
	}

	if node.Normalized != nil {
		result.Normalized = *node.Normalized
		result.HasNormalized = true
	}

	if node.Quality != nil {
		result.Quality = *node.Quality
		result.HasQuality = true
	}

	return result
}

func graphEdgeWire(edge *Edge) *wire.GraphEdgeT {
	if edge == nil {
		return nil
	}

	result := &wire.GraphEdgeT{
		From: edge.From, To: edge.To, Relation: string(edge.Relation),
		Weight: edge.Weight, Confidence: edge.Confidence, Evidence: edge.Evidence,
		ObservedFrom: timeNano(edge.ObservedFrom), Horizon: int64(edge.Horizon),
		At: timeNano(edge.At), Reason: edge.Reason,
	}

	if edge.Quality != nil {
		result.Quality = *edge.Quality
		result.HasQuality = true
	}

	return result
}

func metadataWire(metadata map[string]any) *wire.GraphMetadataT {
	result := &wire.GraphMetadataT{}
	names := make([]string, 0, len(metadata))

	for name := range metadata {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		switch value := metadata[name].(type) {
		case float64:
			result.Numbers = append(result.Numbers, &wire.NamedNumberT{Name: name, Value: value})
		case int:
			result.Numbers = append(result.Numbers, &wire.NamedNumberT{Name: name, Value: float64(value)})
		case string:
			result.Strings = append(result.Strings, &wire.NamedStringT{Name: name, Value: value})
		case bool:
			result.Bools = append(result.Bools, &wire.NamedBoolT{Name: name, Value: value})
		case []string:
			result.StringLists = append(result.StringLists, &wire.NamedStringsT{Name: name, Values: value})
		default:
			panic(fmt.Sprintf("graph telemetry: unsupported metadata %s (%T)", name, value))
		}
	}

	return result
}
