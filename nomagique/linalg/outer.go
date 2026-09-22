package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
OuterServer calculates vector outer product C = alpha * u * v^T.
*/
type OuterServer struct {
	*runtime.System
	result *mat.Dense
}

func NewOuter(ctx context.Context) *OuterServer {
	server := &OuterServer{
		System: runtime.NewSystem(ctx, "linalg.outer"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *OuterServer) Write(ctx context.Context, call Outer_write) error {
	alpha := call.Args().Alpha()
	vecU, err := call.Args().U()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read vector u", err))
	}

	denseU, err := VectorToVecDense(vecU)

	if err != nil {
		return errnie.Error(err)
	}

	vecV, err := call.Args().V()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read vector v", err))
	}

	denseV, err := VectorToVecDense(vecV)

	if err != nil {
		return errnie.Error(err)
	}

	denseC := mat.NewDense(denseU.Len(), denseV.Len(), nil)
	denseC.Outer(alpha, denseU, denseV)
	server.result = denseC
	return nil
}

/*
Done returns calculated results.
*/
func (server *OuterServer) Done(ctx context.Context, call Outer_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.outer.Done] failed to allocate done results",
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
