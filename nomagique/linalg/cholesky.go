package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CholeskyServer computes Cholesky factorization of symmetric positive-definite matrix A = L * L^T.
*/
type CholeskyServer struct {
	*runtime.System
	result mat.Matrix
}

func NewCholesky(ctx context.Context) *CholeskyServer {
	server := &CholeskyServer{
		System: runtime.NewSystem(ctx, "linalg.cholesky"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *CholeskyServer) Write(ctx context.Context, call Cholesky_write) error {
	matrixA, err := call.Args().A()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrix a", err))
	}

	denseA, err := MatrixToDense(matrixA)

	if err != nil {
		return errnie.Error(err)
	}

	rowsA, colsA := denseA.Dims()

	if rowsA != colsA {
		return errnie.Error(errnie.Err(errnie.Validation, "cholesky factorization requires a square matrix", nil))
	}

	symA := mat.NewSymDense(rowsA, nil)

	for rowIndex := 0; rowIndex < rowsA; rowIndex++ {
		for colIndex := rowIndex; colIndex < rowsA; colIndex++ {
			symA.SetSym(rowIndex, colIndex, denseA.At(rowIndex, colIndex))
		}
	}

	var chol mat.Cholesky
	ok := chol.Factorize(symA)

	if !ok {
		return errnie.Error(errnie.Err(errnie.Internal, "cholesky factorization failed (matrix not positive definite)", nil))
	}

	var denseL mat.TriDense
	chol.LTo(&denseL)
	server.result = &denseL
	return nil
}

/*
Done returns calculated results.
*/
func (server *CholeskyServer) Done(ctx context.Context, call Cholesky_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.cholesky.Done] failed to allocate done results",
			err,
		))
	}

	matrixBuilder, err := results.NewL()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create matrix result builder", err))
	}

	if err := DenseToMatrix(server.result, matrixBuilder); err != nil {
		return errnie.Error(err)
	}
	return nil
}
