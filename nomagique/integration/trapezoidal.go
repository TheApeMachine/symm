package integration

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/integrate"
)

/*
TrapezoidalServer calculates numerical integration using trapezoidal rule.
*/
type TrapezoidalServer struct {
	*runtime.System
	integral float64
}

func NewTrapezoidal(ctx context.Context) *TrapezoidalServer {
	server := &TrapezoidalServer{
		System: runtime.NewSystem(ctx, "integration.trapezoidal"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *TrapezoidalServer) Write(ctx context.Context, call Trapezoidal_write) error {
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

	server.integral = integrate.Trapezoidal(xSlice, ySlice)
	return nil
}

/*
Done returns calculated integration results.
*/
func (server *TrapezoidalServer) Done(ctx context.Context, call Trapezoidal_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[integration.trapezoidal.Done] failed to allocate results", err))
	}
	results.SetIntegral(server.integral)
	return nil
}
