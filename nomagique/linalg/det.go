package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
DetServer calculates determinant of a square matrix.
*/
type DetServer struct {
	*runtime.System
	result float64
}

func NewDet(ctx context.Context) *DetServer {
	server := &DetServer{
		System: runtime.NewSystem(ctx, "linalg.det"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *DetServer) Write(ctx context.Context, call Det_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	server.result = mat.Det(denseA)
	return nil
}

/*
Done returns calculated results.
*/
func (server *DetServer) Done(ctx context.Context, call Det_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.det.Done] failed to allocate done results",
			err,
		))
	}

	results.SetDet(server.result)
	return nil
}
