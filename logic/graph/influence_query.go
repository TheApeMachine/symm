package graph

import (
	"sort"

	"github.com/theapemachine/symm/nomagique/relation"
)

/*
Incoming returns the current Influence edges whose target is the coordinate,
retaining the full Relation statistics.
*/
func (influenceGraph *InfluenceGraph) Incoming(target relation.Coordinate) []*InfluenceEdge {
	return influenceGraph.currentEdges(func(edge *InfluenceEdge) bool {
		return edge.Target == target && edge.Type == EdgeInfluence
	})
}

/*
Outgoing returns the current Influence edges whose source is the coordinate,
retaining the full Relation statistics.
*/
func (influenceGraph *InfluenceGraph) Outgoing(source relation.Coordinate) []*InfluenceEdge {
	return influenceGraph.currentEdges(func(edge *InfluenceEdge) bool {
		return edge.Source == source && edge.Type == EdgeInfluence
	})
}

/*
Relation returns the current Influence edge between source and target, or nil.
*/
func (influenceGraph *InfluenceGraph) Relation(source relation.Coordinate, target relation.Coordinate) *InfluenceEdge {
	return influenceGraph.currentEdge(EdgeInfluence, source, target)
}

/*
History returns the chronological Relation history of one edge, oldest first.
*/
func (influenceGraph *InfluenceGraph) History(source relation.Coordinate, target relation.Coordinate) []*InfluenceEdge {
	return influenceGraph.history(EdgeInfluence, source, target)
}

/*
HistoryOf returns the chronological history of one typed edge, oldest first.
*/
func (influenceGraph *InfluenceGraph) HistoryOf(edgeType EdgeType, source relation.Coordinate, target relation.Coordinate) []*InfluenceEdge {
	return influenceGraph.history(edgeType, source, target)
}

func (influenceGraph *InfluenceGraph) history(edgeType EdgeType, source relation.Coordinate, target relation.Coordinate) []*InfluenceEdge {
	if influenceGraph == nil {
		return nil
	}

	influenceGraph.mu.RLock()
	defer influenceGraph.mu.RUnlock()

	history := influenceGraph.edges[edgeKey{edgeType: edgeType, source: source, target: target}]

	if history == nil {
		return nil
	}

	copied := append([]*InfluenceEdge(nil), history.edges...)
	return copied
}

/*
Candidates returns every structurally scheduled edge with its lifecycle
state. Candidate-but-unavailable remains visible and is never treated as "no
relationship".
*/
func (influenceGraph *InfluenceGraph) Candidates() []CandidateEdge {
	if influenceGraph == nil {
		return nil
	}

	influenceGraph.mu.RLock()
	defer influenceGraph.mu.RUnlock()

	candidates := make([]CandidateEdge, 0, len(influenceGraph.candidates))

	for key, state := range influenceGraph.candidates {
		candidates = append(candidates, CandidateEdge{
			Type:   key.edgeType,
			Source: key.source,
			Target: key.target,
			State:  state,
		})
	}

	sort.Slice(candidates, func(left int, right int) bool {
		leftKey := candidates[left].Source.ID() + candidates[left].Target.ID()
		rightKey := candidates[right].Source.ID() + candidates[right].Target.ID()

		return leftKey < rightKey
	})

	return candidates
}

/*
Nodes returns every coordinate node with retained data.
*/
func (influenceGraph *InfluenceGraph) Nodes() []InfluenceNode {
	if influenceGraph == nil {
		return nil
	}

	influenceGraph.mu.RLock()
	defer influenceGraph.mu.RUnlock()

	nodes := make([]InfluenceNode, 0, len(influenceGraph.nodes))

	for _, node := range influenceGraph.nodes {
		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(left int, right int) bool {
		return nodes[left].Coordinate.ID() < nodes[right].Coordinate.ID()
	})

	return nodes
}

/*
Edges returns every current edge across all types, retaining full Relation
statistics.
*/
func (influenceGraph *InfluenceGraph) Edges() []*InfluenceEdge {
	return influenceGraph.currentEdges(func(edge *InfluenceEdge) bool {
		return true
	})
}

/*
Paths returns all directed paths from source to target along current
Influence edges, bounded by maxDepth. Paths preserve the underlying edge
measurements.
*/
func (influenceGraph *InfluenceGraph) Paths(source relation.Coordinate, target relation.Coordinate, maxDepth int) [][]*InfluenceEdge {
	if influenceGraph == nil || maxDepth < 1 {
		return nil
	}

	influenceGraph.mu.RLock()
	defer influenceGraph.mu.RUnlock()

	current := make(map[edgeKey]*InfluenceEdge)

	for key, history := range influenceGraph.edges {
		if len(history.edges) > 0 {
			current[key] = history.edges[len(history.edges)-1]
		}
	}

	paths := make([][]*InfluenceEdge, 0)
	visited := make(map[relation.Coordinate]bool)
	walk := make([]*InfluenceEdge, 0, maxDepth)

	var search func(node relation.Coordinate, depth int)

	search = func(node relation.Coordinate, depth int) {
		if node == target {
			paths = append(paths, append([]*InfluenceEdge(nil), walk...))
			return
		}

		if depth >= maxDepth || visited[node] {
			return
		}

		visited[node] = true

		for key, edge := range current {
			if key.edgeType != EdgeInfluence || edge.Source != node {
				continue
			}

			walk = append(walk, edge)
			search(edge.Target, depth+1)
			walk = walk[:len(walk)-1]
		}

		visited[node] = false
	}

	search(source, 0)
	return paths
}

/*
FamilyEdges returns the current edges whose Source and Target match the given
structural selectors. It is the reversible rollup view: consumers see the
underlying coordinate-level edges and the aggregation rule is stated (plain
enumeration, no score fusion). Family edges never replace coordinate edges as
the input to causal reasoning.
*/
func (influenceGraph *InfluenceGraph) FamilyEdges(
	source relation.Selector,
	target relation.Selector,
) []*InfluenceEdge {
	return influenceGraph.currentEdges(func(edge *InfluenceEdge) bool {
		return source.Matches(edge.Source) && target.Matches(edge.Target) && edge.Type == EdgeInfluence
	})
}

func (influenceGraph *InfluenceGraph) currentEdges(predicate func(*InfluenceEdge) bool) []*InfluenceEdge {
	if influenceGraph == nil {
		return nil
	}

	influenceGraph.mu.RLock()
	defer influenceGraph.mu.RUnlock()

	edges := make([]*InfluenceEdge, 0, len(influenceGraph.edges))

	for key, history := range influenceGraph.edges {
		if len(history.edges) == 0 {
			continue
		}

		edge := history.edges[len(history.edges)-1]

		if edge.Type != key.edgeType {
			continue
		}

		if predicate(edge) {
			edges = append(edges, edge)
		}
	}

	sort.Slice(edges, func(left int, right int) bool {
		leftKey := edges[left].Source.ID() + edges[left].Target.ID()
		rightKey := edges[right].Source.ID() + edges[right].Target.ID()

		return leftKey < rightKey
	})

	return edges
}
