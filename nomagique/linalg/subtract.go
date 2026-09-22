package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SubtractServer calculates the matrix difference C = A - B.
*/
type SubtractServer struct {
	*runtime.System
	result *mat.Dense
}

func NewSubtract(ctx context.Context) *SubtractServer {
	server := &SubtractServer{
		System: runtime.NewSystem(ctx, "linalg.subtract"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *SubtractServer) Write(ctx context.Context, call Subtract_write) error {
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

	if rowsA != rowsB || colsA != colsB {
		return errnie.Error(errnie.Err(errnie.Validation, "matrix dimensions must match", nil))
	}

	denseC := mat.NewDense(rowsA, colsA, nil)
	denseC.Sub(denseA, denseB)
	server.result = denseC
	return nil
}

/*
Done returns calculated results.
*/
func (server *SubtractServer) Done(ctx context.Context, call Subtract_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.subtract.Done] failed to allocate done results",
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
