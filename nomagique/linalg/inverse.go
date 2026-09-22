package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
InverseServer calculates matrix inverse C = A^-1.
*/
type InverseServer struct {
	*runtime.System
	result *mat.Dense
}

func NewInverse(ctx context.Context) *InverseServer {
	server := &InverseServer{
		System: runtime.NewSystem(ctx, "linalg.inverse"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *InverseServer) Write(ctx context.Context, call Inverse_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	rows, cols := denseA.Dims()

	if rows != cols {
		return errnie.Error(errnie.Err(errnie.Validation, "matrix must be square for inverse", nil))
	}

	denseC := mat.NewDense(rows, cols, nil)
	err = denseC.Inverse(denseA)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "matrix inversion failed", err))
	}

	server.result = denseC
	return nil
}

/*
Done returns calculated results.
*/
func (server *InverseServer) Done(ctx context.Context, call Inverse_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.inverse.Done] failed to allocate done results",
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
