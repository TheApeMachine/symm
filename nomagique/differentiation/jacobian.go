package differentiation

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/diff/fd"
	"gonum.org/v1/gonum/mat"
)

/*
JacobianServer approximates Jacobian matrix of vector-valued function.
*/
type JacobianServer struct {
	*runtime.System
	jacobian []float64
}

func NewJacobian(ctx context.Context) *JacobianServer {
	server := &JacobianServer{
		System: runtime.NewSystem(ctx, "differentiation.jacobian"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and calculation parameters.
*/
func (server *JacobianServer) Write(ctx context.Context, call Jacobian_write) error {
	args := call.Args()
	inDim := int(args.InDim())
	outDim := int(args.OutDim())
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

	if inDim <= 0 {
		inDim = len(xSlice)
	}

	if outDim <= 0 {
		outDim = inDim
	}

	vecFn := func(dst, point []float64) {
		for rowIdx := 0; rowIdx < outDim; rowIdx++ {
			dst[rowIdx] = 0.0
			for colIdx, val := range point {
				coeff := 1.0
				cIdx := rowIdx*inDim + colIdx
				if cIdx < len(coeffs) {
					coeff = coeffs[cIdx]
				}
				dst[rowIdx] += coeff * val
			}
		}
	}

	jacMat := mat.NewDense(outDim, inDim, nil)
	fd.Jacobian(jacMat, vecFn, xSlice, nil)
	server.jacobian = make([]float64, outDim*inDim)

	for rowIdx := 0; rowIdx < outDim; rowIdx++ {
		for colIdx := 0; colIdx < inDim; colIdx++ {
			server.jacobian[rowIdx*inDim+colIdx] = jacMat.At(rowIdx, colIdx)
		}
	}
	return nil
}

/*
Done returns calculated differentiation results.
*/
func (server *JacobianServer) Done(ctx context.Context, call Jacobian_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[differentiation.jacobian.Done] failed to allocate results", err))
	}

	listJacobian, err := results.NewJacobian(int32(len(server.jacobian)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate jacobian list", err))
	}

	for index, item := range server.jacobian {
		listJacobian.Set(index, item)
	}
	return nil
}
