package linalg

import (
	"context"
	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
EigenServer computes eigenvalues and right eigenvectors of square matrix A.
*/
type EigenServer struct {
	*runtime.System
	resultValues  *mat.VecDense
	resultVectors *mat.Dense
}

func NewEigen(ctx context.Context) *EigenServer {
	server := &EigenServer{
		System: runtime.NewSystem(ctx, "linalg.eigen"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *EigenServer) Write(ctx context.Context, call Eigen_write) error {
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
		return errnie.Error(errnie.Err(errnie.Validation, "eigen factorization requires a square matrix", nil))
	}

	var eig mat.Eigen
	ok := eig.Factorize(denseA, mat.EigenRight)

	if !ok {
		return errnie.Error(errnie.Err(errnie.Internal, "eigen factorization failed", nil))
	}

	complexValues := eig.Values(nil)
	realValues := make([]float64, len(complexValues))

	for index, cVal := range complexValues {
		realValues[index] = real(cVal)
	}

	vecValues := mat.NewVecDense(len(realValues), realValues)
	var cDense mat.CDense
	eig.VectorsTo(&cDense)
	denseVectors := mat.NewDense(rowsA, rowsA, nil)

	for rowIndex := 0; rowIndex < rowsA; rowIndex++ {
		for colIndex := 0; colIndex < rowsA; colIndex++ {
			denseVectors.Set(rowIndex, colIndex, real(cDense.At(rowIndex, colIndex)))
		}
	}

	server.resultValues = vecValues
	server.resultVectors = denseVectors
	return nil
}

/*
Done returns calculated results.
*/
func (server *EigenServer) Done(ctx context.Context, call Eigen_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[linalg.eigen.Done] failed to allocate done results",
			err,
		))
	}

	valuesBuilder, err := results.NewValues()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create values builder", err))
	}

	if err := VecDenseToVector(server.resultValues, valuesBuilder); err != nil {
		return errnie.Error(err)
	}

	vectorsBuilder, err := results.NewVectors()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to create vectors builder", err))
	}

	if err := DenseToMatrix(server.resultVectors, vectorsBuilder); err != nil {
		return errnie.Error(err)
	}
	return nil
}
