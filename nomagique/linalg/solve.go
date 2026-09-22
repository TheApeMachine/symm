package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SolveServer solves linear matrix equation A * X = B.
*/
type SolveServer struct {
	*runtime.System
	result *mat.Dense
}

func NewSolve(ctx context.Context) *SolveServer {
	server := &SolveServer{
		System: runtime.NewSystem(ctx, "linalg.solve"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *SolveServer) Write(ctx context.Context, call Solve_write) error {
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

	rowsA, _ := denseA.Dims()
	_, colsB := denseB.Dims()
	denseX := mat.NewDense(rowsA, colsB, nil)
	err = denseX.Solve(denseA, denseB)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "matrix solve failed", err))
	}

	server.result = denseX
	return nil
}

/*
Done returns calculated results.
*/
func (server *SolveServer) Done(ctx context.Context, call Solve_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.solve.Done] failed to allocate done results",
			err,
		))
	}

	matrixBuilder, err := results.NewX()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create matrix result builder", err))
	}

	if err := DenseToMatrix(server.result, matrixBuilder); err != nil {
		return errnie.Error(err)
	}
	return nil
}
