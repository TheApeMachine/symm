/*
Package network provides a lock-free, generic graph data structure built on the
golang.design/x/lockfree skip list.

It is a concurrent, mutable graph that can be updated in place once created —
the driving requirement for the logic stage, whose Step method is an
incrementing observation stream rather than a fresh-per-tick rebuild.

The graph is generic over node identity (ID) and node/edge payloads (N, E). It
is deliberately free of any market, signal, or telemetry semantics: it knows
only about node identity, directionality, weight, and traversal. All domain
meaning lives in the caller's payload types.

Storage: nodes are one lock-free ordered map (SkipList) keyed by ID; edges are a
second lock-free ordered map keyed by source ID, each value an outgoing
adjacency list. Point reads (Node, HasNode, Outgoing) and writes (SetNode,
SetEdge) are lock-free and allocation-lean; ordered iteration is exposed through
Range, which observes a weakly-consistent snapshot under concurrent mutation.
*/
package network

import (
	"golang.design/x/lockfree"
	"golang.design/x/lockfree/lf"
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
Graph is a lock-free, mutable adjacency structure backed by skip lists.

Directionality: it stores directed edges (From → To). Edge weight and direction
are the caller's semantics; the graph preserves them and provides adjacency and
traversal over them.
*/
type Graph[ID comparable, N any, E any] struct {
	less  lockfree.Less[ID]
	nodes *lf.SkipList[ID, N]
	edges *lf.SkipList[ID, []Edge[ID, E]]
}

/*
NewGraph builds an empty graph. less orders node identities deterministically;
it must be a strict weak order.
*/
func NewGraph[ID comparable, N any, E any](less lockfree.Less[ID]) *Graph[ID, N, E] {
	return &Graph[ID, N, E]{
		less:  less,
		nodes: lf.NewSkipList[ID, N](less),
		edges: lf.NewSkipList[ID, []Edge[ID, E]](less),
	}
}

/*
SetNode inserts or updates a node in place. O(log n).
*/
func (graph *Graph[ID, N, E]) SetNode(node Node[ID, N]) {
	graph.nodes.Set(node.ID, node.Data)
}

/*
HasNode reports whether a node exists. Wait-free.
*/
func (graph *Graph[ID, N, E]) HasNode(id ID) bool {
	return graph.nodes.Search(id)
}

/*
Node returns a node and whether it exists. Wait-free.
*/
func (graph *Graph[ID, N, E]) Node(id ID) (Node[ID, N], bool) {
	data, found := graph.nodes.Get(id)
	return Node[ID, N]{ID: id, Data: data}, found
}

/*
SetEdge inserts a directed edge From → To, updating any existing From → To edge
in place while preserving direction and weight. Reversing direction requires a
separate SetEdge call. It updates exactly the From node's outgoing list and
records both endpoints as nodes.
*/
func (graph *Graph[ID, N, E]) SetEdge(edge Edge[ID, E]) {
	current, _ := graph.edges.Get(edge.From)
	graph.edges.Set(edge.From, upsertOutgoing(current, edge))

	var zero N
	graph.nodes.Set(edge.From, zero)
	graph.nodes.Set(edge.To, zero)
}

/*
Outgoing returns the edges whose source is id, as a copy safe to mutate.
O(degree).
*/
func (graph *Graph[ID, N, E]) Outgoing(id ID) []Edge[ID, E] {
	current, _ := graph.edges.Get(id)

	if current == nil {
		return nil
	}

	return append([]Edge[ID, E](nil), current...)
}

/*
Range invokes op for every node whose key is in [from, to), in ascending order.
It is the caller's ordered-iteration primitive; the caller supplies the bounds,
so a full walk passes a to that sorts after every key (the tail sentinel ends
the walk when to sorts after the last real key).
*/
func (graph *Graph[ID, N, E]) Range(from ID, to ID, op func(Node[ID, N])) {
	graph.nodes.Range(from, to, func(id ID, data N) {
		op(Node[ID, N]{ID: id, Data: data})
	})
}

/*
RangeEdges invokes op for every edge whose source is in [from, to), in ascending
source order. The caller supplies the bounds exactly as with Range.
*/
func (graph *Graph[ID, N, E]) RangeEdges(from ID, to ID, op func(Edge[ID, E])) {
	graph.edges.Range(from, to, func(id ID, edges []Edge[ID, E]) {
		for _, edge := range edges {
			op(edge)
		}
	})
}

/*
Len returns the approximate number of nodes.
*/
func (graph *Graph[ID, N, E]) Len() int {
	return graph.nodes.Len()
}

// upsertOutgoing inserts or replaces an edge (matched by To) in an outgoing list.
func upsertOutgoing[ID comparable, E any](list []Edge[ID, E], edge Edge[ID, E]) []Edge[ID, E] {
	for index := range list {
		if list[index].To == edge.To {
			list[index] = edge
			return list
		}
	}

	return append(list, edge)
}
