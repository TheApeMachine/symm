package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
VecAddServer calculates vector sum w = u + v.
*/
type VecAddServer struct {
	*runtime.System
	result *mat.VecDense
}

func NewVecAdd(ctx context.Context) *VecAddServer {
	server := &VecAddServer{
		System: runtime.NewSystem(ctx, "linalg.vec_add"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *VecAddServer) Write(ctx context.Context, call VecAdd_write) error {
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

	if denseU.Len() != denseV.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, "vector lengths must match for addition", nil))
	}

	denseW := mat.NewVecDense(denseU.Len(), nil)
	denseW.AddVec(denseU, denseV)
	server.result = denseW
	return nil
}

/*
Done returns calculated results.
*/
func (server *VecAddServer) Done(ctx context.Context, call VecAdd_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.vec_add.Done] failed to allocate done results",
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
