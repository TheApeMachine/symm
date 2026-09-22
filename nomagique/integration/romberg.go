package integration

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/integrate"
)

/*
RombergServer calculates numerical integration using Romberg's method.
*/
type RombergServer struct {
	*runtime.System
	integral float64
}

func NewRomberg(ctx context.Context) *RombergServer {
	server := &RombergServer{
		System: runtime.NewSystem(ctx, "integration.romberg"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *RombergServer) Write(ctx context.Context, call Romberg_write) error {
	args := call.Args()
	dxVal := args.Dx()
	yList, err := args.Y()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read y", err))
	}

	if dxVal <= 0 {
		dxVal = 1.0
	}

	lenVal := yList.Len()
	if lenVal < 3 {
		server.integral = 0.0
		return nil
	}

	countVal := lenVal - 1
	powK := 1
	for (1 << (powK + 1)) <= countVal {
		powK++
	}
	validLen := (1 << powK) + 1
	ySlice := make([]float64, validLen)

	for index := 0; index < validLen; index++ {
		ySlice[index] = yList.At(index)
	}

	server.integral = integrate.Romberg(ySlice, dxVal)
	return nil
}

/*
Done returns calculated integration results.
*/
func (server *RombergServer) Done(ctx context.Context, call Romberg_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[integration.romberg.Done] failed to allocate results", err))
	}
	results.SetIntegral(server.integral)
	return nil
}
