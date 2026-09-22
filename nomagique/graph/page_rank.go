package graph

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/network"
)

/*
PageRankServer calculates PageRank link-analysis centrality score.
*/
type PageRankServer struct {
	*runtime.System
	nodes []int64
	ranks []float64
}

func NewPageRank(ctx context.Context) *PageRankServer {
	server := &PageRankServer{
		System: runtime.NewSystem(ctx, "graph.page_rank"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and graph operations.
*/
func (server *PageRankServer) Write(ctx context.Context, call PageRank_write) error {
	args := call.Args()
	fromList, err := args.FromNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read fromNodes", err))
	}

	toList, err := args.ToNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read toNodes", err))
	}

	dampVal := args.Damping()
	tolVal := args.Tol()
	if dampVal <= 0 {
		dampVal = 0.85
	}

	if tolVal <= 0 {
		tolVal = 1e-6
	}

	graphVal := simple.NewDirectedGraph()
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

		edge := graphVal.NewEdge(simple.Node(uID), simple.Node(vID))
		graphVal.SetEdge(edge)
	}

	rankMap := network.PageRank(graphVal, dampVal, tolVal)
	server.nodes = nil
	server.ranks = nil

	for nodeID, rankVal := range rankMap {
		server.nodes = append(server.nodes, nodeID)
		server.ranks = append(server.ranks, rankVal)
	}
	return nil
}

/*
Done returns calculated graph analysis results.
*/
func (server *PageRankServer) Done(ctx context.Context, call PageRank_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[graph.page_rank.Done] failed to allocate results", err))
	}

	listNodes, err := results.NewNodes(int32(len(server.nodes)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate nodes list", err))
	}

	for index, item := range server.nodes {
		listNodes.Set(index, item)
	}

	listRanks, err := results.NewRanks(int32(len(server.ranks)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate ranks list", err))
	}

	for index, item := range server.ranks {
		listRanks.Set(index, item)
	}
	return nil
}
