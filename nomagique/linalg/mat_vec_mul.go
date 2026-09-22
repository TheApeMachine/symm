package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MatVecMulServer calculates matrix-vector multiplication y = A * x.
*/
type MatVecMulServer struct {
	*runtime.System
	result *mat.VecDense
}

func NewMatVecMul(ctx context.Context) *MatVecMulServer {
	server := &MatVecMulServer{
		System: runtime.NewSystem(ctx, "linalg.mat_vec_mul"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MatVecMulServer) Write(ctx context.Context, call MatVecMul_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	vecX, err := call.Args().X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read vector x", err))
	}

	denseX, err := VectorToVecDense(vecX)

	if err != nil {
		return errnie.Error(err)
	}

	rowsA, colsA := denseA.Dims()

	if colsA != denseX.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, "matrix columns must match vector length", nil))
	}

	denseY := mat.NewVecDense(rowsA, nil)
	denseY.MulVec(denseA, denseX)
	server.result = denseY
	return nil
}

/*
Done returns calculated results.
*/
func (server *MatVecMulServer) Done(ctx context.Context, call MatVecMul_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.mat_vec_mul.Done] failed to allocate done results",
			err,
		))
	}

	vecBuilder, err := results.NewY()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create vector result builder", err))
	}

	if err := VecDenseToVector(server.result, vecBuilder); err != nil {
		return errnie.Error(err)
	}
	return nil
}
