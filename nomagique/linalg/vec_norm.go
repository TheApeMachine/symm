package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
VecNormServer calculates vector norm of order ord.
*/
type VecNormServer struct {
	*runtime.System
	result float64
}

func NewVecNorm(ctx context.Context) *VecNormServer {
	server := &VecNormServer{
		System: runtime.NewSystem(ctx, "linalg.vec_norm"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *VecNormServer) Write(ctx context.Context, call VecNorm_write) error {
	ord := call.Args().Ord()
	vecU, err := call.Args().U()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read vector u", err))
	}

	denseU, err := VectorToVecDense(vecU)

	if err != nil {
		return errnie.Error(err)
	}

	if ord == 0 {
		ord = 2
	}

	server.result = mat.Norm(denseU, ord)
	return nil
}

/*
Done returns calculated results.
*/
func (server *VecNormServer) Done(ctx context.Context, call VecNorm_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.vec_norm.Done] failed to allocate done results",
			err,
		))
	}

	results.SetNorm(server.result)
	return nil
}
