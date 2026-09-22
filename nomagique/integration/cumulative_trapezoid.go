package integration

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CumulativeTrapezoidServer calculates running cumulative trapezoidal integral curve.
*/
type CumulativeTrapezoidServer struct {
	*runtime.System
	cumulative []float64
	total      float64
}

func NewCumulativeTrapezoid(ctx context.Context) *CumulativeTrapezoidServer {
	server := &CumulativeTrapezoidServer{
		System: runtime.NewSystem(ctx, "integration.cumulative_trapezoid"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *CumulativeTrapezoidServer) Write(ctx context.Context, call CumulativeTrapezoid_write) error {
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

	if lenVal == 0 {
		server.cumulative = nil
		server.total = 0.0
		return nil
	}

	xSlice := make([]float64, lenVal)
	ySlice := make([]float64, lenVal)
	for index := 0; index < lenVal; index++ {
		xSlice[index] = xList.At(index)
		ySlice[index] = yList.At(index)
	}

	cumulative := make([]float64, lenVal)
	cumulative[0] = 0.0
	for index := 1; index < lenVal; index++ {
		diffX := xSlice[index] - xSlice[index-1]
		trap := 0.5 * (ySlice[index] + ySlice[index-1]) * diffX
		cumulative[index] = cumulative[index-1] + trap
	}
	server.cumulative = cumulative
	server.total = cumulative[lenVal-1]
	return nil
}

/*
Done returns calculated integration results.
*/
func (server *CumulativeTrapezoidServer) Done(ctx context.Context, call CumulativeTrapezoid_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[integration.cumulative_trapezoid.Done] failed to allocate results", err))
	}

	listCumulative, err := results.NewCumulative(int32(len(server.cumulative)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate cumulative list", err))
	}

	for index, item := range server.cumulative {
		listCumulative.Set(index, item)
	}
	results.SetTotal(server.total)
	return nil
}
