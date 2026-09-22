package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NormServer calculates matrix norm of order ord.
*/
type NormServer struct {
	*runtime.System
	result float64
}

func NewNorm(ctx context.Context) *NormServer {
	server := &NormServer{
		System: runtime.NewSystem(ctx, "linalg.norm"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *NormServer) Write(ctx context.Context, call Norm_write) error {
	ord := call.Args().Ord()
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	if ord == 0 {
		ord = 2
	}

	server.result = mat.Norm(denseA, ord)
	return nil
}

/*
Done returns calculated results.
*/
func (server *NormServer) Done(ctx context.Context, call Norm_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.norm.Done] failed to allocate done results",
			err,
		))
	}

	results.SetNorm(server.result)
	return nil
}
