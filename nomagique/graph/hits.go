package graph

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/simple"
)

/*
HITSServer calculates Kleinberg HITS hub and authority centrality scores.
*/
type HITSServer struct {
	*runtime.System
	nodes       []int64
	hubs        []float64
	authorities []float64
}

func NewHITS(ctx context.Context) *HITSServer {
	server := &HITSServer{
		System: runtime.NewSystem(ctx, "graph.hits"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and graph operations.
*/
func (server *HITSServer) Write(ctx context.Context, call HITS_write) error {
	args := call.Args()
	fromList, err := args.FromNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read fromNodes", err))
	}

	toList, err := args.ToNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read toNodes", err))
	}

	tolVal := args.Tol()
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

	hitsMap := network.HITS(graphVal, tolVal)
	server.nodes = nil
	server.hubs = nil
	server.authorities = nil

	for nodeID, hubAuth := range hitsMap {
		server.nodes = append(server.nodes, nodeID)
		server.hubs = append(server.hubs, hubAuth.Hub)
		server.authorities = append(server.authorities, hubAuth.Authority)
	}
	return nil
}

/*
Done returns calculated graph analysis results.
*/
func (server *HITSServer) Done(ctx context.Context, call HITS_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[graph.hits.Done] failed to allocate results", err))
	}

	listNodes, err := results.NewNodes(int32(len(server.nodes)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate nodes list", err))
	}

	for index, item := range server.nodes {
		listNodes.Set(index, item)
	}

	listHubs, err := results.NewHubs(int32(len(server.hubs)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate hubs list", err))
	}

	for index, item := range server.hubs {
		listHubs.Set(index, item)
	}

	listAuthorities, err := results.NewAuthorities(int32(len(server.authorities)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate authorities list", err))
	}

	for index, item := range server.authorities {
		listAuthorities.Set(index, item)
	}
	return nil
}
