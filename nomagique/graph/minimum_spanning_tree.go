package graph

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/path"
)

/*
MinimumSpanningTreeServer calculates minimum spanning tree of graph using Kruskal algorithm.
*/
type MinimumSpanningTreeServer struct {
	*runtime.System
	totalWeight float64
	mstFrom []int64
	mstTo []int64
}

func NewMinimumSpanningTree(ctx context.Context) *MinimumSpanningTreeServer {
	server := &MinimumSpanningTreeServer{
		System: runtime.NewSystem(ctx, "graph.minimum_spanning_tree"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and graph operations.
*/
func (server *MinimumSpanningTreeServer) Write(ctx context.Context, call MinimumSpanningTree_write) error {
	args := call.Args()
	fromList, err := args.FromNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read fromNodes", err))
	}

	toList, err := args.ToNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read toNodes", err))
	}
	weightList, err := args.Weights()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read weights", err))
	}

	graphVal := simple.NewWeightedUndirectedGraph(0, 0)
	lenVal := fromList.Len()
	if toList.Len() < lenVal {
		lenVal = toList.Len()
	}

	for index := 0; index < lenVal; index++ {
		uID := fromList.At(index)
		vID := toList.At(index)
		if graphVal.Node(uID) == nil {
			graphVal.AddNode(simple.Node(uID))
		}

		if graphVal.Node(vID) == nil {
			graphVal.AddNode(simple.Node(vID))
		}

		weight := 1.0
		if index < weightList.Len() {
			weight = weightList.At(index)
		}

		edge := graphVal.NewWeightedEdge(simple.Node(uID), simple.Node(vID), weight)
		graphVal.SetWeightedEdge(edge)
	}

	mstGraph := simple.NewWeightedUndirectedGraph(0, 0)
	server.totalWeight = path.Kruskal(mstGraph, graphVal)
	edgesIt := mstGraph.WeightedEdges()
	server.mstFrom = nil
	server.mstTo = nil

	for edgesIt.Next() {
		edge := edgesIt.WeightedEdge()
		server.mstFrom = append(server.mstFrom, edge.From().ID())
		server.mstTo = append(server.mstTo, edge.To().ID())
	}
	return nil
}

/*
Done returns calculated graph analysis results.
*/
func (server *MinimumSpanningTreeServer) Done(ctx context.Context, call MinimumSpanningTree_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[graph.minimum_spanning_tree.Done] failed to allocate results", err))
	}
	results.SetTotalWeight(server.totalWeight)

	listMstFrom, err := results.NewMstFrom(int32(len(server.mstFrom)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate mstFrom list", err))
	}

	for index, item := range server.mstFrom {
		listMstFrom.Set(index, item)
	}

	listMstTo, err := results.NewMstTo(int32(len(server.mstTo)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate mstTo list", err))
	}

	for index, item := range server.mstTo {
		listMstTo.Set(index, item)
	}
	return nil
}
