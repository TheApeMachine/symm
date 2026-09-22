package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MultiplyServer calculates the matrix product C = A * B.
*/
type MultiplyServer struct {
	*runtime.System
	result *mat.Dense
}

func NewMultiply(ctx context.Context) *MultiplyServer {
	server := &MultiplyServer{
		System: runtime.NewSystem(ctx, "linalg.multiply"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MultiplyServer) Write(ctx context.Context, call Multiply_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	matrixB, err := call.Args().B()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix b", err))
	}

	denseB, err := MatrixToDense(matrixB)

	if err != nil {
		return errnie.Error(err)
	}

	rowsA, colsA := denseA.Dims()
	rowsB, colsB := denseB.Dims()

	if colsA != rowsB {
		return errnie.Error(errnie.Err(errnie.Validation, "inner dimensions must match for matrix multiplication", nil))
	}

	denseC := mat.NewDense(rowsA, colsB, nil)
	denseC.Mul(denseA, denseB)
	server.result = denseC
	return nil
}

/*
Done returns calculated results.
*/
func (server *MultiplyServer) Done(ctx context.Context, call Multiply_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.multiply.Done] failed to allocate done results",
			err,
		))
	}

	matrixBuilder, err := results.NewC()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create matrix result builder", err))
	}

	if err := DenseToMatrix(server.result, matrixBuilder); err != nil {
		return errnie.Error(err)
	}
	return nil
}
