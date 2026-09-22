package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SolveVecServer solves linear system equation A * x = b.
*/
type SolveVecServer struct {
	*runtime.System
	result *mat.VecDense
}

func NewSolveVec(ctx context.Context) *SolveVecServer {
	server := &SolveVecServer{
		System: runtime.NewSystem(ctx, "linalg.solve_vec"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *SolveVecServer) Write(ctx context.Context, call SolveVec_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	vecB, err := call.Args().B()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read vector b", err))
	}

	denseB, err := VectorToVecDense(vecB)

	if err != nil {
		return errnie.Error(err)
	}

	rowsA, _ := denseA.Dims()
	denseX := mat.NewVecDense(rowsA, nil)
	err = denseX.SolveVec(denseA, denseB)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "solve vector failed", err))
	}

	server.result = denseX
	return nil
}

/*
Done returns calculated results.
*/
func (server *SolveVecServer) Done(ctx context.Context, call SolveVec_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.solve_vec.Done] failed to allocate done results",
			err,
		))
	}

	vecBuilder, err := results.NewX()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create vector result builder", err))
	}

	if err := VecDenseToVector(server.result, vecBuilder); err != nil {
		return errnie.Error(err)
	}
	return nil
}
