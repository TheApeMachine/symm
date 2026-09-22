package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
QRServer computes QR factorization of matrix A = Q * R.
*/
type QRServer struct {
	*runtime.System
	resultQ *mat.Dense
	resultR *mat.Dense
}

func NewQR(ctx context.Context) *QRServer {
	server := &QRServer{
		System: runtime.NewSystem(ctx, "linalg.qr"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *QRServer) Write(ctx context.Context, call QR_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	rowsA, colsA := denseA.Dims()
	var qr mat.QR
	qr.Factorize(denseA)

	denseQ := mat.NewDense(rowsA, rowsA, nil)
	qr.QTo(denseQ)

	denseR := mat.NewDense(rowsA, colsA, nil)
	qr.RTo(denseR)

	server.resultQ = denseQ
	server.resultR = denseR
	return nil
}

/*
Done returns calculated results.
*/
func (server *QRServer) Done(ctx context.Context, call QR_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.qr.Done] failed to allocate done results",
			err,
		))
	}

	qBuilder, err := results.NewQ()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create q builder", err))
	}

	if err := DenseToMatrix(server.resultQ, qBuilder); err != nil {
		return errnie.Error(err)
	}

	rBuilder, err := results.NewR()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create r builder", err))
	}

	if err := DenseToMatrix(server.resultR, rBuilder); err != nil {
		return errnie.Error(err)
	}
	return nil
}
