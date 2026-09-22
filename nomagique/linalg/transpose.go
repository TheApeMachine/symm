package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TransposeServer calculates matrix transpose C = A^T.
*/
type TransposeServer struct {
	*runtime.System
	result *mat.Dense
}

func NewTranspose(ctx context.Context) *TransposeServer {
	server := &TransposeServer{
		System: runtime.NewSystem(ctx, "linalg.transpose"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *TransposeServer) Write(ctx context.Context, call Transpose_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	server.result = mat.DenseCopyOf(denseA.T())
	return nil
}

/*
Done returns calculated results.
*/
func (server *TransposeServer) Done(ctx context.Context, call Transpose_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.transpose.Done] failed to allocate done results",
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
