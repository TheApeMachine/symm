package differentiation

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/diff/fd"
	"gonum.org/v1/gonum/mat"
)

/*
HessianServer approximates Hessian matrix of scalar field via finite differences.
*/
type HessianServer struct {
	*runtime.System
	hessian []float64
}

func NewHessian(ctx context.Context) *HessianServer {
	server := &HessianServer{
		System: runtime.NewSystem(ctx, "differentiation.hessian"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *HessianServer) Write(ctx context.Context, call Hessian_write) error {
	args := call.Args()
	dimVal := int(args.Dim())
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

	if dimVal <= 0 {
		dimVal = len(xSlice)
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

	hessMat := mat.NewSymDense(dimVal, nil)
	fd.Hessian(hessMat, scalarFn, xSlice, nil)
	server.hessian = make([]float64, dimVal*dimVal)

	for rowIdx := 0; rowIdx < dimVal; rowIdx++ {
		for colIdx := 0; colIdx < dimVal; colIdx++ {
			server.hessian[rowIdx*dimVal+colIdx] = hessMat.At(rowIdx, colIdx)
		}
	}
	return nil
}

/*
Done returns calculated differentiation results.
*/
func (server *HessianServer) Done(ctx context.Context, call Hessian_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[differentiation.hessian.Done] failed to allocate results", err))
	}

	listHessian, err := results.NewHessian(int32(len(server.hessian)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate hessian list", err))
	}

	for index, item := range server.hessian {
		listHessian.Set(index, item)
	}
	return nil
}
