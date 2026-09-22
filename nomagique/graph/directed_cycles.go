package graph

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

/*
DirectedCyclesServer detects cycles in a directed graph.
*/
type DirectedCyclesServer struct {
	*runtime.System
	cycleCount int32
	hasCycles  bool
}

func NewDirectedCycles(ctx context.Context) *DirectedCyclesServer {
	server := &DirectedCyclesServer{
		System: runtime.NewSystem(ctx, "graph.directed_cycles"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and graph operations.
*/
func (server *DirectedCyclesServer) Write(ctx context.Context, call DirectedCycles_write) error {
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

	cycles := topo.DirectedCyclesIn(graphVal)
	server.cycleCount = int32(len(cycles))
	server.hasCycles = len(cycles) > 0
	return nil
}

/*
Done returns calculated graph analysis results.
*/
func (server *DirectedCyclesServer) Done(ctx context.Context, call DirectedCycles_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[graph.directed_cycles.Done] failed to allocate results", err))
	}
	results.SetCycleCount(server.cycleCount)
	results.SetHasCycles(server.hasCycles)
	return nil
}
