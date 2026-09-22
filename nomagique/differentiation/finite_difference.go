package differentiation

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
FiniteDifferenceServer calculates discrete forward, central, and backward differences.
*/
type FiniteDifferenceServer struct {
	*runtime.System
	forward []float64
	central []float64
	backward []float64
}

func NewFiniteDifference(ctx context.Context) *FiniteDifferenceServer {
	server := &FiniteDifferenceServer{
		System: runtime.NewSystem(ctx, "differentiation.finite_difference"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *FiniteDifferenceServer) Write(ctx context.Context, call FiniteDifference_write) error {
	args := call.Args()
	stepVal := args.Step()
	valList, err := args.Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read values", err))
	}

	if stepVal <= 0 {
		stepVal = 1.0
	}

	vals := make([]float64, valList.Len())
	for index := 0; index < valList.Len(); index++ {
		vals[index] = valList.At(index)
	}

	lenVals := len(vals)
	if lenVals >= 2 {
		server.forward = make([]float64, lenVals-1)
		server.backward = make([]float64, lenVals-1)
		for index := 0; index < lenVals-1; index++ {
			server.forward[index] = (vals[index+1] - vals[index]) / stepVal
			server.backward[index] = (vals[index+1] - vals[index]) / stepVal
		}
	}

	if lenVals >= 3 {
		server.central = make([]float64, lenVals-2)
		for index := 1; index < lenVals-1; index++ {
			server.central[index-1] = (vals[index+1] - vals[index-1]) / (2.0 * stepVal)
		}
	}
	return nil
}

/*
Done returns calculated differentiation results.
*/
func (server *FiniteDifferenceServer) Done(ctx context.Context, call FiniteDifference_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[differentiation.finite_difference.Done] failed to allocate results", err))
	}

	listForward, err := results.NewForward(int32(len(server.forward)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate forward list", err))
	}

	for index, item := range server.forward {
		listForward.Set(index, item)
	}

	listCentral, err := results.NewCentral(int32(len(server.central)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate central list", err))
	}

	for index, item := range server.central {
		listCentral.Set(index, item)
	}

	listBackward, err := results.NewBackward(int32(len(server.backward)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate backward list", err))
	}

	for index, item := range server.backward {
		listBackward.Set(index, item)
	}
	return nil
}
