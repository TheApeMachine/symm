package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
AddServer calculates the matrix sum C = A + B.
*/
type AddServer struct {
	*runtime.System
	result *mat.Dense
}

func NewAdd(ctx context.Context) *AddServer {
	server := &AddServer{
		System: runtime.NewSystem(ctx, "linalg.add"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *AddServer) Write(ctx context.Context, call Add_write) error {
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
	denseC.Add(denseA, denseB)
	server.result = denseC
	return nil
}

/*
Done returns calculated results.
*/
func (server *AddServer) Done(ctx context.Context, call Add_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.add.Done] failed to allocate done results",
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
