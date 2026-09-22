package differentiation

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/diff/fd"
)

/*
LaplacianServer approximates Laplacian of scalar field.
*/
type LaplacianServer struct {
	*runtime.System
	laplacian float64
}

func NewLaplacian(ctx context.Context) *LaplacianServer {
	server := &LaplacianServer{
		System: runtime.NewSystem(ctx, "differentiation.laplacian"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *LaplacianServer) Write(ctx context.Context, call Laplacian_write) error {
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

	server.laplacian = fd.Laplacian(scalarFn, xSlice, nil)
	return nil
}

/*
Done returns calculated differentiation results.
*/
func (server *LaplacianServer) Done(ctx context.Context, call Laplacian_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[differentiation.laplacian.Done] failed to allocate results", err))
	}
	results.SetLaplacian(server.laplacian)
	return nil
}
