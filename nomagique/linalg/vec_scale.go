package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
VecScaleServer calculates scalar vector multiplication w = alpha * u.
*/
type VecScaleServer struct {
	*runtime.System
	result *mat.VecDense
}

func NewVecScale(ctx context.Context) *VecScaleServer {
	server := &VecScaleServer{
		System: runtime.NewSystem(ctx, "linalg.vec_scale"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *VecScaleServer) Write(ctx context.Context, call VecScale_write) error {
	alpha := call.Args().Alpha()
	vecU, err := call.Args().U()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read vector u", err))
	}

	denseU, err := VectorToVecDense(vecU)

	if err != nil {
		return errnie.Error(err)
	}

	denseW := mat.NewVecDense(denseU.Len(), nil)
	denseW.ScaleVec(alpha, denseU)
	server.result = denseW
	return nil
}

/*
Done returns calculated results.
*/
func (server *VecScaleServer) Done(ctx context.Context, call VecScale_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.vec_scale.Done] failed to allocate done results",
			err,
		))
	}

	vecBuilder, err := results.NewW()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create vector result builder", err))
	}

	if err := VecDenseToVector(server.result, vecBuilder); err != nil {
		return errnie.Error(err)
	}
	return nil
}
