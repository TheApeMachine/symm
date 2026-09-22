package graph

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/simple"
)

/*
BetweennessServer calculates betweenness centrality for routing bottleneck detection.
*/
type BetweennessServer struct {
	*runtime.System
	nodes  []int64
	scores []float64
}

func NewBetweenness(ctx context.Context) *BetweennessServer {
	server := &BetweennessServer{
		System: runtime.NewSystem(ctx, "graph.betweenness"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and graph operations.
*/
func (server *BetweennessServer) Write(ctx context.Context, call Betweenness_write) error {
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

	bwMap := network.Betweenness(graphVal)
	server.nodes = nil
	server.scores = nil

	for nodeID, scoreVal := range bwMap {
		server.nodes = append(server.nodes, nodeID)
		server.scores = append(server.scores, scoreVal)
	}
	return nil
}

/*
Done returns calculated graph analysis results.
*/
func (server *BetweennessServer) Done(ctx context.Context, call Betweenness_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[graph.betweenness.Done] failed to allocate results", err))
	}

	listNodes, err := results.NewNodes(int32(len(server.nodes)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate nodes list", err))
	}

	for index, item := range server.nodes {
		listNodes.Set(index, item)
	}

	listScores, err := results.NewScores(int32(len(server.scores)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate scores list", err))
	}

	for index, item := range server.scores {
		listScores.Set(index, item)
	}
	return nil
}
