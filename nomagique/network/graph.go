package network

import (
	"fmt"
	"iter"
	"unsafe"

	"golang.design/x/lockfree"
	"golang.design/x/lockfree/lf"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

type Node struct {
	ID   float64
	Data string
}

type Edge struct {
	From   float64
	To     float64
	Weight float64
	Data   string
}

type Command struct {
	Op     string
	ID     float64
	From   float64
	To     float64
	Weight float64
	Data   string
}

type Result struct {
	Count float64
	Len   float64
	Found float64
	Data  string
	Nodes []Node
	Edges []Edge
}

/*
Graph is a lock-free, mutable adjacency Primitive backed by skip lists.
*/
type Graph struct {
	*core.PrimitiveError
	nodes *lf.SkipList[float64, string]
	edges *lf.SkipList[float64, []Edge]
}

/*
NewGraph builds an empty graph Primitive.
*/
func NewGraph(less lockfree.Less[float64]) core.Primitive {
	op := &Graph{
		PrimitiveError: core.NewPrimitiveError(),
	}

	if less == nil {
		op.Error(fmt.Errorf("%w: network: graph requires an ordering function", core.ErrShape))
		return op
	}

	op.nodes = lf.NewSkipList[float64, string](less)
	op.edges = lf.NewSkipList[float64, []Edge](less)

	return op
}

func (op *Graph) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.Error() != nil {
		return func(func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			cmd := (*Command)(arriving)

			if cmd == nil {
				op.Error(core.ErrShape)
				return
			}

			result := &Result{}

			switch cmd.Op {
			case "set_node":
				op.nodes.Set(cmd.ID, cmd.Data)
				result.Len = float64(op.nodes.Len())

			case "set_edge":
				current, _ := op.edges.Get(cmd.From)
				replaced := false

				for index := range current {
					if current[index].To == cmd.To {
						current[index] = Edge{
							From:   cmd.From,
							To:     cmd.To,
							Weight: cmd.Weight,
							Data:   cmd.Data,
						}
						replaced = true
						break
					}
				}

				if !replaced {
					current = append(current, Edge{
						From:   cmd.From,
						To:     cmd.To,
						Weight: cmd.Weight,
						Data:   cmd.Data,
					})
				}

				op.edges.Set(cmd.From, current)
				op.nodes.Set(cmd.From, "")
				op.nodes.Set(cmd.To, "")
				result.Len = float64(op.nodes.Len())

			case "get_node":
				payload, found := op.nodes.Get(cmd.ID)

				if found {
					result.Found = 1.0
					result.Data = payload
				}

			case "outgoing":
				current, _ := op.edges.Get(cmd.ID)
				result.Count = float64(len(current))
				result.Edges = current

			case "range_nodes":
				var nodes []Node
				op.nodes.Range(cmd.From, cmd.To, func(id float64, payload string) {
					nodes = append(nodes, Node{ID: id, Data: payload})
				})
				result.Nodes = nodes
				result.Count = float64(len(nodes))

			case "range_edges":
				var edges []Edge
				op.edges.Range(cmd.From, cmd.To, func(id float64, list []Edge) {
					edges = append(edges, list...)
				})
				result.Edges = edges
				result.Count = float64(len(edges))

			case "len":
				result.Len = float64(op.nodes.Len())

			default:
				op.Error(fmt.Errorf("%w: network: unknown graph op %q", core.ErrShape, cmd.Op))
				return
			}

			for value := range data.NewValue(unsafe.Pointer(result)).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
