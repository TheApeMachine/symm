package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
DotServer calculates vector dot product s = u . v.
*/
type DotServer struct {
	*runtime.System
	result float64
}

func NewDot(ctx context.Context) *DotServer {
	server := &DotServer{
		System: runtime.NewSystem(ctx, "linalg.dot"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *DotServer) Write(ctx context.Context, call Dot_write) error {
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
		return errnie.Error(errnie.Err(errnie.Validation, "vector lengths must match for dot product", nil))
	}

	server.result = mat.Dot(denseU, denseV)
	return nil
}

/*
Done returns calculated results.
*/
func (server *DotServer) Done(ctx context.Context, call Dot_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.dot.Done] failed to allocate done results",
			err,
		))
	}

	results.SetDot(server.result)
	return nil
}
