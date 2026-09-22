package differentiation

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/diff/fd"
)

/*
GradientServer approximates gradient vector of scalar field via finite differences.
*/
type GradientServer struct {
	*runtime.System
	gradient []float64
}

func NewGradient(ctx context.Context) *GradientServer {
	server := &GradientServer{
		System: runtime.NewSystem(ctx, "differentiation.gradient"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *GradientServer) Write(ctx context.Context, call Gradient_write) error {
	args := call.Args()
	coeffList, err := args.Coeffs()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read coeffs", err))
	}

	xList, err := args.X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read x", err))
	}

	coeffs := make([]float64, coeffList.Len())
	for index := 0; index < coeffList.Len(); index++ {
		coeffs[index] = coeffList.At(index)
	}

	xSlice := make([]float64, xList.Len())
	for index := 0; index < xList.Len(); index++ {
		xSlice[index] = xList.At(index)
	}

	scalarFn := func(point []float64) float64 {
		res := 0.0
		for index, val := range point {
			coeff := 1.0
			if index < len(coeffs) {
				coeff = coeffs[index]
			}
			res += coeff * val * val
		}
		return res
	}

	server.gradient = fd.Gradient(nil, scalarFn, xSlice, nil)
	return nil
}

/*
Done returns calculated differentiation results.
*/
func (server *GradientServer) Done(ctx context.Context, call Gradient_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[differentiation.gradient.Done] failed to allocate results", err))
	}

	listGradient, err := results.NewGradient(int32(len(server.gradient)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate gradient list", err))
	}

	for index, item := range server.gradient {
		listGradient.Set(index, item)
	}
	return nil
}
