package graph

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

/*
ConnectedComponentsServer identifies connected components in an undirected graph.
*/
type ConnectedComponentsServer struct {
	*runtime.System
	componentCount int32
	componentSizes []int64
	members        []int64
	memberOf       []int64
}

func NewConnectedComponents(ctx context.Context) *ConnectedComponentsServer {
	server := &ConnectedComponentsServer{
		System: runtime.NewSystem(ctx, "graph.connected_components"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and graph operations.
*/
func (server *ConnectedComponentsServer) Write(ctx context.Context, call ConnectedComponents_write) error {
	args := call.Args()
	fromList, err := args.FromNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read fromNodes", err))
	}

	toList, err := args.ToNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read toNodes", err))
	}

	graphVal := simple.NewUndirectedGraph()
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

	compList := topo.ConnectedComponents(graphVal)
	server.componentCount = int32(len(compList))
	server.componentSizes = make([]int64, len(compList))
	server.members = server.members[:0]
	server.memberOf = server.memberOf[:0]

	for index, compItem := range compList {
		server.componentSizes[index] = int64(len(compItem))

		for _, held := range compItem {
			server.members = append(server.members, held.ID())
			server.memberOf = append(server.memberOf, int64(index))
		}
	}
	return nil
}

/*
Done returns calculated graph analysis results.
*/
func (server *ConnectedComponentsServer) Done(ctx context.Context, call ConnectedComponents_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[graph.connected_components.Done] failed to allocate results", err))
	}
	results.SetComponentCount(server.componentCount)

	listComponentSizes, err := results.NewComponentSizes(int32(len(server.componentSizes)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate componentSizes list", err))
	}

	for index, item := range server.componentSizes {
		listComponentSizes.Set(index, item)
	}

	if len(server.members) == 0 {
		return nil
	}

	members, err := results.NewMembers(int32(len(server.members)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate members list", err))
	}

	memberOf, err := results.NewMemberOf(int32(len(server.memberOf)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate memberOf list", err))
	}

	for index := range server.members {
		members.Set(index, server.members[index])
		memberOf.Set(index, server.memberOf[index])
	}
	return nil
}
