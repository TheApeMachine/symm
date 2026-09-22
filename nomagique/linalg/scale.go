package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ScaleServer calculates scalar matrix multiplication C = alpha * A.
*/
type ScaleServer struct {
	*runtime.System
	result *mat.Dense
}

func NewScale(ctx context.Context) *ScaleServer {
	server := &ScaleServer{
		System: runtime.NewSystem(ctx, "linalg.scale"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *ScaleServer) Write(ctx context.Context, call Scale_write) error {
	alpha := call.Args().Alpha()
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	rows, cols := denseA.Dims()
	denseC := mat.NewDense(rows, cols, nil)
	denseC.Scale(alpha, denseA)
	server.result = denseC
	return nil
}

/*
Done returns calculated results.
*/
func (server *ScaleServer) Done(ctx context.Context, call Scale_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.scale.Done] failed to allocate done results",
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
