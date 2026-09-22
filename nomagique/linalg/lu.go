package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
LUServer computes LU decomposition of matrix A with partial pivoting: P * A = L * U.
*/
type LUServer struct {
	*runtime.System
	resultL mat.Matrix
	resultU mat.Matrix
}

func NewLU(ctx context.Context) *LUServer {
	server := &LUServer{
		System: runtime.NewSystem(ctx, "linalg.lu"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *LUServer) Write(ctx context.Context, call LU_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}
	var lu mat.LU
	lu.Factorize(denseA)

	var denseL mat.TriDense
	lu.LTo(&denseL)
	var denseU mat.TriDense
	lu.UTo(&denseU)

	server.resultL = &denseL
	server.resultU = &denseU
	return nil
}

/*
Done returns calculated results.
*/
func (server *LUServer) Done(ctx context.Context, call LU_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.lu.Done] failed to allocate done results",
			err,
		))
	}

	lBuilder, err := results.NewL()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create l builder", err))
	}

	if err := DenseToMatrix(server.resultL, lBuilder); err != nil {
		return errnie.Error(err)
	}

	uBuilder, err := results.NewU()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create u builder", err))
	}

	if err := DenseToMatrix(server.resultU, uBuilder); err != nil {
		return errnie.Error(err)
	}
	return nil
}
