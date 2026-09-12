/*
Package network provides a lock-free, generic graph Primitive built on the
golang.design/x/lockfree skip list.

Everything is a streaming Primitive over an unsafe.Pointer wire: the graph
receives command payloads that discriminate one operation (mutate a node or
edge, query a node, list outgoing edges, walk a key span, or count nodes) and
yields one result payload per command. The graph is therefore a concurrent,
mutable structure updated in place across a stream of commands — the driving
requirement for the logic stage, whose Step method is an incrementing
observation stream rather than a fresh-per-tick rebuild.

The graph is generic over node identity (ID) and node/edge payloads (N, E). It
is deliberately free of any market, signal, or telemetry semantics: it knows
only about node identity, directionality, weight, and traversal. All domain
meaning lives in the caller's payload types.

Storage: nodes are one lock-free ordered map (SkipList) keyed by ID; edges are
a second lock-free ordered map keyed by source ID, each value an outgoing
adjacency list. Point reads and writes are lock-free and allocation-lean;
ordered iteration observes a weakly-consistent snapshot under concurrent
mutation.
*/
package network

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"golang.design/x/lockfree"
	"golang.design/x/lockfree/lf"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Node is one vertex in the network. ID is the caller's identifier; Data is the
caller's arbitrary payload (never inspected by the graph).
*/
type Node[ID comparable, N any] struct {
	ID   ID
	Data N
}

/*
Edge is one directed, weighted link between two node identities. Weight is the
caller's numeric strength; both its magnitude and sign are preserved and never
interpreted by the graph.
*/
type Edge[ID comparable, E any] struct {
	From   ID
	To     ID
	Weight float64
	Data   E
}

/*
Span is the half-open [From, To) key range of an ordered walk. A full walk
passes a To that sorts after every key.
*/
type Span[ID comparable] struct {
	From ID
	To   ID
}

/*
Count asks for the approximate number of nodes.
*/
type Count struct{}

/*
GraphCommand discriminates one graph operation. Exactly one field is set;
anything else is a shape failure.
*/
type GraphCommand[ID comparable, N any, E any] struct {
	SetNode    *Node[ID, N]
	SetEdge    *Edge[ID, E]
	Node       *ID
	Outgoing   *ID
	RangeNodes *Span[ID]
	RangeEdges *Span[ID]
	Len        *Count
}

/*
GraphResult is one command response: the node lookup (with existence), a copy
of one node's outgoing edges, the nodes or edges of an ordered walk, or the
node count. Mutations acknowledge with the post-mutation node count.
*/
type GraphResult[ID comparable, N any, E any] struct {
	Node     Node[ID, N]
	Found    bool
	Outgoing []Edge[ID, E]
	Nodes    []Node[ID, N]
	Edges    []Edge[ID, E]
	Len      int
}

/*
Graph is a lock-free, mutable adjacency Primitive backed by skip lists.

Directionality: it stores directed edges (From → To). Edge weight and direction
are the caller's semantics; the graph preserves them and provides adjacency
and traversal over them.
*/
type Graph[ID comparable, N any, E any] struct {
	err   error
	nodes *lf.SkipList[ID, N]
	edges *lf.SkipList[ID, []Edge[ID, E]]
	out   GraphResult[ID, N, E]
}

/*
NewGraph builds an empty graph Primitive. less orders node identities
deterministically; it must be a strict weak order. A nil less function is
recorded as a shape failure and every stream over the graph yields nothing.
*/
func NewGraph[ID comparable, N any, E any](less lockfree.Less[ID]) core.Primitive {
	if less == nil {
		return &Graph[ID, N, E]{
			err: fmt.Errorf("%w: network: graph requires an ordering function", core.ErrShape),
		}
	}

	return &Graph[ID, N, E]{
		nodes: lf.NewSkipList[ID, N](less),
		edges: lf.NewSkipList[ID, []Edge[ID, E]](less),
	}
}

/*
Next receives *GraphCommand payloads and yields a *GraphResult for each.
Invalid commands end the stream with the error recorded.
*/
func (op *Graph[ID, N, E]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*GraphCommand[ID, N, E])(arriving)
			result, err := op.execute(command)

			if err != nil {
				op.Error(err)
				return
			}

			op.out = result

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Graph[ID, N, E]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
execute dispatches one command to its intent and returns its result.
*/
func (op *Graph[ID, N, E]) execute(
	command *GraphCommand[ID, N, E],
) (GraphResult[ID, N, E], error) {
	intents := 0

	if command.SetNode != nil {
		intents++
	}

	if command.SetEdge != nil {
		intents++
	}

	if command.Node != nil {
		intents++
	}

	if command.Outgoing != nil {
		intents++
	}

	if command.RangeNodes != nil {
		intents++
	}

	if command.RangeEdges != nil {
		intents++
	}

	if command.Len != nil {
		intents++
	}

	if intents != 1 {
		return GraphResult[ID, N, E]{}, fmt.Errorf(
			"%w: network: graph command must set exactly one intent",
			core.ErrShape,
		)
	}

	if command.SetNode != nil {
		return op.setNode(*command.SetNode), nil
	}

	if command.SetEdge != nil {
		return op.setEdge(*command.SetEdge), nil
	}

	if command.Node != nil {
		return op.node(*command.Node), nil
	}

	if command.Outgoing != nil {
		return op.outgoing(*command.Outgoing), nil
	}

	if command.RangeNodes != nil {
		return op.rangeNodes(*command.RangeNodes), nil
	}

	if command.RangeEdges != nil {
		return op.rangeEdges(*command.RangeEdges), nil
	}

	return GraphResult[ID, N, E]{Len: op.nodes.Len()}, nil
}

/*
setNode inserts or updates a node in place. O(log n).
*/
func (op *Graph[ID, N, E]) setNode(node Node[ID, N]) GraphResult[ID, N, E] {
	op.nodes.Set(node.ID, node.Data)

	return GraphResult[ID, N, E]{Len: op.nodes.Len()}
}

/*
setEdge inserts a directed edge From → To, updating any existing From → To edge
in place while preserving direction and weight. Reversing direction requires a
separate SetEdge command. It updates exactly the From node's outgoing list and
records both endpoints as nodes.
*/
func (op *Graph[ID, N, E]) setEdge(edge Edge[ID, E]) GraphResult[ID, N, E] {
	current, _ := op.edges.Get(edge.From)
	op.edges.Set(edge.From, upsertOutgoing(current, edge))

	var zero N
	op.nodes.Set(edge.From, zero)
	op.nodes.Set(edge.To, zero)

	return GraphResult[ID, N, E]{Len: op.nodes.Len()}
}

/*
node returns the node and whether it exists. Wait-free.
*/
func (op *Graph[ID, N, E]) node(id ID) GraphResult[ID, N, E] {
	data, found := op.nodes.Get(id)

	return GraphResult[ID, N, E]{
		Node:  Node[ID, N]{ID: id, Data: data},
		Found: found,
	}
}

/*
outgoing returns the edges whose source is id, as a copy safe to mutate.
O(degree).
*/
func (op *Graph[ID, N, E]) outgoing(id ID) GraphResult[ID, N, E] {
	current, _ := op.edges.Get(id)

	if current == nil {
		return GraphResult[ID, N, E]{Outgoing: nil}
	}

	return GraphResult[ID, N, E]{
		Outgoing: append([]Edge[ID, E](nil), current...),
	}
}

/*
rangeNodes collects every node whose key is in [From, To), in ascending order.
*/
func (op *Graph[ID, N, E]) rangeNodes(span Span[ID]) GraphResult[ID, N, E] {
	result := GraphResult[ID, N, E]{Nodes: []Node[ID, N](nil)}
	op.nodes.Range(span.From, span.To, func(id ID, data N) {
		result.Nodes = append(result.Nodes, Node[ID, N]{ID: id, Data: data})
	})

	return result
}

/*
rangeEdges collects every edge whose source is in [From, To), in ascending
source order.
*/
func (op *Graph[ID, N, E]) rangeEdges(span Span[ID]) GraphResult[ID, N, E] {
	result := GraphResult[ID, N, E]{Edges: []Edge[ID, E](nil)}
	op.edges.Range(span.From, span.To, func(id ID, edges []Edge[ID, E]) {
		result.Edges = append(result.Edges, edges...)
	})

	return result
}

/*
upsertOutgoing inserts or replaces an edge (matched by To) in an outgoing list.
*/
func upsertOutgoing[ID comparable, E any](list []Edge[ID, E], edge Edge[ID, E]) []Edge[ID, E] {
	for index := range list {
		if list[index].To == edge.To {
			list[index] = edge
			return list
		}
	}

	return append(list, edge)
}
