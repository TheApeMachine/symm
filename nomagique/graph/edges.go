package graph

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
EdgesServer reads a square matrix of relationships as the graph it describes.

Only the upper triangle is read. A relationship is symmetric — that two
quantities are related is one fact, not two — so reading both halves would
double every edge and count every strength twice.
*/
type EdgesServer struct {
	*runtime.System
	from    []int64
	to      []int64
	weights []float64
	pairs   int
}

func NewEdges(ctx context.Context) *EdgesServer {
	server := &EdgesServer{
		System: runtime.NewSystem(ctx, "graph.edges"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write reads the matrix and keeps the pairs that reach the cut.
*/
func (server *EdgesServer) Write(ctx context.Context, call Edges_write) error {
	dim := int(call.Args().Dim())

	matrix, err := call.Args().Matrix()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"graph.edges: failed to read the matrix",
			err,
		))
	}

	if !matrix.IsValid() || matrix.Len() == 0 {
		server.from, server.to, server.weights, server.pairs = nil, nil, nil, 0
		return nil
	}

	if dim <= 0 || matrix.Len() != dim*dim {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"graph.edges: the matrix is not the square its dimension claims",
			nil,
		))
	}

	threshold := call.Args().Threshold()
	signed := call.Args().Signed()

	server.from = server.from[:0]
	server.to = server.to[:0]
	server.weights = server.weights[:0]
	server.pairs = 0

	for row := range dim {
		for column := row + 1; column < dim; column++ {
			server.pairs++
			strength := matrix.At(row*dim + column)

			// An inverse relationship is a relationship. Reading strength
			// unsigned keeps a pair that moves consistently opposite, which
			// discarding the sign of the cut alone would throw away.
			if !signed {
				strength = math.Abs(strength)
			}

			if strength < threshold {
				continue
			}

			server.from = append(server.from, int64(row))
			server.to = append(server.to, int64(column))
			server.weights = append(server.weights, strength)
		}
	}

	return nil
}

/*
Done hands back the graph the matrix described, and how much of it survived
the cut.
*/
func (server *EdgesServer) Done(ctx context.Context, call Edges_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"graph.edges: failed to allocate results",
			err,
		))
	}

	results.SetCount(int32(len(server.from)))

	// How much of what was measured the cut kept. A graph that kept
	// everything and one that kept almost nothing are different readings of
	// the same matrix, and nothing downstream can tell them apart from the
	// edges alone.
	if server.pairs > 0 {
		results.SetKept(float64(len(server.from)) / float64(server.pairs))
	}

	if len(server.from) == 0 {
		return nil
	}

	fromNodes, err := results.NewFromNodes(int32(len(server.from)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"graph.edges: failed to allocate the producing ends",
			err,
		))
	}

	toNodes, err := results.NewToNodes(int32(len(server.to)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"graph.edges: failed to allocate the consuming ends",
			err,
		))
	}

	weights, err := results.NewWeights(int32(len(server.weights)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"graph.edges: failed to allocate the strengths",
			err,
		))
	}

	for index := range server.from {
		fromNodes.Set(index, server.from[index])
		toNodes.Set(index, server.to[index])
		weights.Set(index, server.weights[index])
	}

	return nil
}
