package graph

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/path"
)

/*
ShortestPathServer calculates shortest path between source and target nodes using Dijkstra algorithm.
*/
type ShortestPathServer struct {
	*runtime.System
	path []int64
	weight float64
}

func NewShortestPath(ctx context.Context) *ShortestPathServer {
	server := &ShortestPathServer{
		System: runtime.NewSystem(ctx, "graph.shortest_path"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and graph operations.
*/
func (server *ShortestPathServer) Write(ctx context.Context, call ShortestPath_write) error {
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

	srcID := args.Source()
	tgtID := args.Target()
	graphVal := simple.NewWeightedDirectedGraph(0, 0)
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

	if graphVal.Node(srcID) == nil {
		graphVal.AddNode(simple.Node(srcID))
	}

	if graphVal.Node(tgtID) == nil {
		graphVal.AddNode(simple.Node(tgtID))
	}

	shortest := path.DijkstraFrom(simple.Node(srcID), graphVal)
	pathNodes, totalWeight := shortest.To(tgtID)
	server.weight = totalWeight
	server.path = make([]int64, len(pathNodes))

	for index, nodeItem := range pathNodes {
		server.path[index] = nodeItem.ID()
	}
	return nil
}

/*
Done returns calculated graph analysis results.
*/
func (server *ShortestPathServer) Done(ctx context.Context, call ShortestPath_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[graph.shortest_path.Done] failed to allocate results", err))
	}

	listPath, err := results.NewPath(int32(len(server.path)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate path list", err))
	}

	for index, item := range server.path {
		listPath.Set(index, item)
	}
	results.SetWeight(server.weight)
	return nil
}
