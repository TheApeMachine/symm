package strategy

import (
	"math"

	"github.com/theapemachine/symm/types"
)

type CategoryNode struct {
	Category types.CategoryType
	Score    float64
}

type CategoryEdge struct {
	From   types.CategoryType
	To     types.CategoryType
	Weight float64
}

type Graph struct {
	nodes []CategoryNode
	edges []CategoryEdge
}

func NewGraph(
	nodes []CategoryNode,
	edges []CategoryEdge,
) *Graph {
	return &Graph{
		nodes: nodes,
		edges: edges,
	}
}

func (graph *Graph) Walk() float64 {
	if graph == nil {
		return 0
	}

	scores := map[types.CategoryType]float64{}

	for _, node := range graph.nodes {
		if !finite(node.Score) {
			return 0
		}

		scores[node.Category] += node.Score
	}

	for _, edge := range graph.edges {
		if !finite(edge.Weight) {
			return 0
		}

		scores[edge.To] += scores[edge.From] * edge.Weight
	}

	return scores[types.ForecastEdge]
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
