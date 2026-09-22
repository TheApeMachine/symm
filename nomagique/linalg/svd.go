package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SVDServer computes Singular Value Decomposition A = U * S * V^T.
*/
type SVDServer struct {
	*runtime.System
	resultU *mat.Dense
	resultS *mat.VecDense
	resultV *mat.Dense
}

func NewSVD(ctx context.Context) *SVDServer {
	server := &SVDServer{
		System: runtime.NewSystem(ctx, "linalg.svd"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *SVDServer) Write(ctx context.Context, call SVD_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	rowsA, colsA := denseA.Dims()
	var svd mat.SVD
	ok := svd.Factorize(denseA, mat.SVDThin)

	if !ok {
		return errnie.Error(errnie.Err(errnie.Internal, "svd factorization failed", nil))
	}

	minDim := min(rowsA, colsA)
	denseU := mat.NewDense(rowsA, minDim, nil)
	svd.UTo(denseU)

	denseV := mat.NewDense(colsA, minDim, nil)
	svd.VTo(denseV)

	singularValues := svd.Values(nil)
	vecS := mat.NewVecDense(len(singularValues), singularValues)

	server.resultU = denseU
	server.resultS = vecS
	server.resultV = denseV
	return nil
}

/*
Done returns calculated results.
*/
func (server *SVDServer) Done(ctx context.Context, call SVD_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.svd.Done] failed to allocate done results",
			err,
		))
	}

	uBuilder, err := results.NewU()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create u builder", err))
	}

	if err := DenseToMatrix(server.resultU, uBuilder); err != nil {
		return errnie.Error(err)
	}

	sBuilder, err := results.NewS()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create s builder", err))
	}

	if err := VecDenseToVector(server.resultS, sBuilder); err != nil {
		return errnie.Error(err)
	}

	vBuilder, err := results.NewV()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create v builder", err))
	}

	if err := DenseToMatrix(server.resultV, vBuilder); err != nil {
		return errnie.Error(err)
	}
	return nil
}
