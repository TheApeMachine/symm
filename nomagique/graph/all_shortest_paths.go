package graph

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/path"
)

/*
AllShortestPathsServer calculates all-pairs shortest paths across weighted graph.
*/
type AllShortestPathsServer struct {
	*runtime.System
	distances []float64
	nodeCount int32
}

func NewAllShortestPaths(ctx context.Context) *AllShortestPathsServer {
	server := &AllShortestPathsServer{
		System: runtime.NewSystem(ctx, "graph.all_shortest_paths"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and graph operations.
*/
func (server *AllShortestPathsServer) Write(ctx context.Context, call AllShortestPaths_write) error {
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

	allPaths := path.DijkstraAllPaths(graphVal)
	nodesIt := graphVal.Nodes()
	var nodeIDs []int64
	for nodesIt.Next() {
		nodeIDs = append(nodeIDs, nodesIt.Node().ID())
	}

	count := len(nodeIDs)
	server.nodeCount = int32(count)
	server.distances = make([]float64, count*count)

	for rowIdx, uID := range nodeIDs {
		for colIdx, vID := range nodeIDs {
			server.distances[rowIdx*count+colIdx] = allPaths.Weight(uID, vID)
		}
	}
	return nil
}

/*
Done returns calculated graph analysis results.
*/
func (server *AllShortestPathsServer) Done(ctx context.Context, call AllShortestPaths_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[graph.all_shortest_paths.Done] failed to allocate results", err))
	}

	listDistances, err := results.NewDistances(int32(len(server.distances)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate distances list", err))
	}

	for index, item := range server.distances {
		listDistances.Set(index, item)
	}
	results.SetNodeCount(server.nodeCount)
	return nil
}
