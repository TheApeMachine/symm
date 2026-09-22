package integration

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/integrate"
)

/*
SimpsonsServer calculates numerical integration using Simpson's rule.
*/
type SimpsonsServer struct {
	*runtime.System
	integral float64
}

func NewSimpsons(ctx context.Context) *SimpsonsServer {
	server := &SimpsonsServer{
		System: runtime.NewSystem(ctx, "integration.simpsons"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *SimpsonsServer) Write(ctx context.Context, call Simpsons_write) error {
	args := call.Args()
	xList, err := args.X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read x", err))
	}

	yList, err := args.Y()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read y", err))
	}

	lenVal := xList.Len()
	if yList.Len() < lenVal {
		lenVal = yList.Len()
	}

	xSlice := make([]float64, lenVal)
	ySlice := make([]float64, lenVal)

	for index := 0; index < lenVal; index++ {
		xSlice[index] = xList.At(index)
		ySlice[index] = yList.At(index)
	}

	server.integral = integrate.Simpsons(xSlice, ySlice)
	return nil
}

/*
Done returns calculated integration results.
*/
func (server *SimpsonsServer) Done(ctx context.Context, call Simpsons_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[integration.simpsons.Done] failed to allocate results", err))
	}
	results.SetIntegral(server.integral)
	return nil
}
