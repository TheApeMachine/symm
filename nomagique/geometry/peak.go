package geometry

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Peak identifies density peaks on the spanning forest and assigns root basins.
Nodes climb along edges toward higher-authority neighbors; local maximums become basin roots.
*/
type Peak struct {
	*core.PrimitiveError
}

func NewPeak() *Peak {
	return &Peak{PrimitiveError: core.NewPrimitiveError()}
}

func (peak *Peak) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var edges []*Edge
		nodeIndex := make(map[core.Primitive]int)
		authorities := make(map[core.Primitive]float64)

		for arriving := range in {
			if peak.Error() != nil {
				return
			}

			if arriving == nil {
				continue
			}

			edge := (*Edge)(arriving)
			edges = append(edges, edge)

			if _, exists := nodeIndex[edge.Left]; !exists && edge.Left != nil {
				nodeIndex[edge.Left] = len(nodeIndex)
				authorities[edge.Left] = edge.Authority[0]
			}

			if _, exists := nodeIndex[edge.Right]; !exists && edge.Right != nil {
				nodeIndex[edge.Right] = len(nodeIndex)
				authorities[edge.Right] = edge.Authority[1]
			}
		}

		if len(edges) == 0 {
			return
		}

		parent := make(map[core.Primitive]core.Primitive)
		for node := range nodeIndex {
			parent[node] = node
		}

		for _, edge := range edges {
			lAuth := authorities[edge.Left]
			rAuth := authorities[edge.Right]

			if lAuth > rAuth && lAuth > authorities[parent[edge.Right]] {
				parent[edge.Right] = edge.Left
			}
			if rAuth > lAuth && rAuth > authorities[parent[edge.Left]] {
				parent[edge.Left] = edge.Right
			}
		}

		var root func(core.Primitive) core.Primitive
		root = func(p core.Primitive) core.Primitive {
			if parent[p] == p {
				return p
			}
			parent[p] = root(parent[p])
			return parent[p]
		}

		for _, edge := range edges {
			edge.Basin[0] = nodeIndex[root(edge.Left)]
			edge.Basin[1] = nodeIndex[root(edge.Right)]

			if !yield(unsafe.Pointer(edge)) {
				return
			}
		}
	}
}
