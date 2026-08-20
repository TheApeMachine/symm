package types

import wire "github.com/theapemachine/symm/telemetry/generated/telemetry"

func reasoningWire(topology ReasoningTopology) *wire.ReasoningT {
	nodes := make([]*wire.ReasoningNodeT, 0, len(topology.Nodes))

	for _, node := range topology.Nodes {
		nodes = append(nodes, &wire.ReasoningNodeT{
			Id: node.ID, Label: node.Label, Symbol: node.Symbol,
			Tier: string(node.Tier), Role: node.Role, Source: node.Source,
			Value: node.Value, Confidence: node.Confidence, Derived: node.Derived,
		})
	}

	links := make([]*wire.ReasoningLinkT, 0, len(topology.Links))

	for _, link := range topology.Links {
		links = append(links, &wire.ReasoningLinkT{
			From: link.From, To: link.To, Relation: link.Relation,
			Weight: link.Weight, Confidence: link.Confidence, Derived: link.Derived,
		})
	}

	return &wire.ReasoningT{
		Symbol: topology.Symbol, Ready: topology.Ready, Reason: topology.Reason,
		ObservedRows: int64(topology.ObservedRows), MaximumHorizon: int64(topology.MaximumHorizon),
		Treatment: topology.Treatment, Mediator: topology.Mediator, Target: topology.Target,
		Controls: topology.Controls, CurrentState: namedNumbers(topology.CurrentState),
		Nodes: nodes, Links: links,
	}
}
