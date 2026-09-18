package strategy

import (
	"iter"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
)

type GridTopologyNode struct {
	Coordinate *geometry.Coordinate `json:"coordinate"`
	X          float64              `json:"x"`
	Y          float64              `json:"y"`
	Basin      int                  `json:"basin"`
	Authority  float64              `json:"authority"`
}

type GridTopologyEdge struct {
	FromX    int     `json:"fromX"`
	FromY    int     `json:"fromY"`
	ToX      int     `json:"toX"`
	ToY      int     `json:"toY"`
	Distance float64 `json:"distance"`
	IsBorder bool    `json:"isBorder"`
}

type GridTopologySnapshot struct {
	Nodes []GridTopologyNode `json:"nodes"`
	Edges []GridTopologyEdge `json:"edges"`
}

type GridTopologyCollector[T core.Ordered[T]] struct {
	*core.PrimitiveError
	snapshot atomic.Pointer[GridTopologySnapshot]
}

func NewGridTopologyCollector[T core.Ordered[T]]() *GridTopologyCollector[T] {
	return &GridTopologyCollector[T]{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (c *GridTopologyCollector[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var edges []*geometry.Edge
		for arriving := range in {
			if arriving == nil {
				continue
			}
			edge := (*geometry.Edge)(arriving)
			edges = append(edges, edge)
			if !yield(arriving) {
				return
			}
		}

		if len(edges) == 0 {
			return
		}

		snap := &GridTopologySnapshot{
			Nodes: make([]GridTopologyNode, 0),
			Edges: make([]GridTopologyEdge, 0, len(edges)),
		}

		seenNodes := make(map[core.Primitive]struct{})

		for _, e := range edges {
			var fromX, fromY, toX, toY int
			if cLeft, ok := e.Left.(*geometry.Coordinate); ok {
				fromX, fromY = cLeft.X, cLeft.Y
				if _, seen := seenNodes[e.Left]; !seen {
					seenNodes[e.Left] = struct{}{}
					snap.Nodes = append(snap.Nodes, GridTopologyNode{
						Coordinate: cLeft,
						X:          float64(cLeft.X),
						Y:          float64(cLeft.Y),
						Basin:      e.Basin[0],
						Authority:  e.Authority[0],
					})
				}
			}

			if cRight, ok := e.Right.(*geometry.Coordinate); ok {
				toX, toY = cRight.X, cRight.Y
				if _, seen := seenNodes[e.Right]; !seen {
					seenNodes[e.Right] = struct{}{}
					snap.Nodes = append(snap.Nodes, GridTopologyNode{
						Coordinate: cRight,
						X:          float64(cRight.X),
						Y:          float64(cRight.Y),
						Basin:      e.Basin[1],
						Authority:  e.Authority[1],
					})
				}
			}

			snap.Edges = append(snap.Edges, GridTopologyEdge{
				FromX:    fromX,
				FromY:    fromY,
				ToX:      toX,
				ToY:      toY,
				Distance: e.Distance,
				IsBorder: e.Border,
			})
		}

		c.snapshot.Store(snap)
	}
}

func (c *GridTopologyCollector[T]) Snapshot() *GridTopologySnapshot {
	return c.snapshot.Load()
}
