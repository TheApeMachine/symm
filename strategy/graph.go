package strategy

import "github.com/theapemachine/symm/types"

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

func (graph *Graph) Walk() {

}
