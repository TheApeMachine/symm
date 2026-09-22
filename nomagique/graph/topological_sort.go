package graph

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

/*
TopologicalSortServer calculates topological ordering of a directed acyclic graph.
*/
type TopologicalSortServer struct {
	*runtime.System
	order    []int64
	hasCycle bool
}

func NewTopologicalSort(ctx context.Context) *TopologicalSortServer {
	server := &TopologicalSortServer{
		System: runtime.NewSystem(ctx, "graph.topological_sort"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and graph operations.
*/
func (server *TopologicalSortServer) Write(ctx context.Context, call TopologicalSort_write) error {
	args := call.Args()
	fromList, err := args.FromNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read fromNodes", err))
	}

	toList, err := args.ToNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read toNodes", err))
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

	orderNodes, err := topo.Sort(graphVal)
	if err != nil {
		server.hasCycle = true
		server.order = nil
		return nil
	}

	server.hasCycle = false
	server.order = make([]int64, len(orderNodes))
	for index, nodeItem := range orderNodes {
		server.order[index] = nodeItem.ID()
	}
	return nil
}

/*
Done returns calculated graph analysis results.
*/
func (server *TopologicalSortServer) Done(ctx context.Context, call TopologicalSort_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[graph.topological_sort.Done] failed to allocate results", err))
	}

	listOrder, err := results.NewOrder(int32(len(server.order)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate order list", err))
	}

	for index, item := range server.order {
		listOrder.Set(index, item)
	}
	results.SetHasCycle(server.hasCycle)
	return nil
}
