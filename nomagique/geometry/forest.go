package geometry

import (
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Forest constructs the Euclidean Minimum Spanning Forest across sympathetic edges,
yielding the spanning tree edges that connect the nodes.
*/
type Forest struct {
	*core.PrimitiveError
}

func NewForest() *Forest {
	return &Forest{PrimitiveError: core.NewPrimitiveError()}
}

func (forest *Forest) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var edges []*Edge

		for arriving := range in {
			if forest.Error() != nil {
				return
			}

			if arriving == nil {
				continue
			}

			edges = append(edges, (*Edge)(arriving))
		}

		if len(edges) == 0 {
			return
		}

		// Sort edges by Distance ascending (Kruskal's algorithm)
		slices.SortFunc(edges, func(a, b *Edge) int {
			if a.Distance < b.Distance {
				return -1
			}
			if a.Distance > b.Distance {
				return 1
			}
			return 0
		})

		parent := make(map[core.Primitive]core.Primitive)
		var find func(core.Primitive) core.Primitive
		find = func(p core.Primitive) core.Primitive {
			if parent[p] == nil || parent[p] == p {
				parent[p] = p
				return p
			}
			parent[p] = find(parent[p])
			return parent[p]
		}

		for _, edge := range edges {
			if edge.Left == nil || edge.Right == nil {
				continue
			}

			rootLeft := find(edge.Left)
			rootRight := find(edge.Right)

			if rootLeft != rootRight {
				parent[rootLeft] = rootRight
				if !yield(unsafe.Pointer(edge)) {
					return
				}
			}
		}
	}
}
