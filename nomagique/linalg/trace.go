package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TraceServer calculates trace of a square matrix.
*/
type TraceServer struct {
	*runtime.System
	result float64
}

func NewTrace(ctx context.Context) *TraceServer {
	server := &TraceServer{
		System: runtime.NewSystem(ctx, "linalg.trace"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *TraceServer) Write(ctx context.Context, call Trace_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	server.result = mat.Trace(denseA)
	return nil
}

/*
Done returns calculated results.
*/
func (server *TraceServer) Done(ctx context.Context, call Trace_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.trace.Done] failed to allocate done results",
			err,
		))
	}

	results.SetTrace(server.result)
	return nil
}
