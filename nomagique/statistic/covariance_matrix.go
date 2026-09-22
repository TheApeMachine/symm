package statistic

import (
	"context"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CovarianceMatrixServer calculates sample covariance matrix of multi-column observation matrix.
*/
type CovarianceMatrixServer struct {
	*runtime.System
	resultMatrix []float64
	dim int32
}

func NewCovarianceMatrix(ctx context.Context) *CovarianceMatrixServer {
	server := &CovarianceMatrixServer{
		System: runtime.NewSystem(ctx, "statistic.covariance_matrix"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *CovarianceMatrixServer) Write(ctx context.Context, call CovarianceMatrix_write) error {
	rows := int(call.Args().Rows())
	cols := int(call.Args().Cols())
	dataList, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read data", err))
	}

	expectedLen := rows * cols

	if dataList.Len() != expectedLen || rows <= 0 || cols <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "invalid matrix dimensions", nil))
	}

	slice := make([]float64, expectedLen)

	for index := 0; index < expectedLen; index++ {
		slice[index] = dataList.At(index)
	}

	dense := mat.NewDense(rows, cols, slice)
	cov := mat.NewSymDense(cols, nil)
	stat.CovarianceMatrix(cov, dense, nil)

	outSlice := make([]float64, cols*cols)

	for r := 0; r < cols; r++ {
		for c := 0; c < cols; c++ {
			outSlice[r*cols+c] = cov.At(r, c)
		}
	}

	server.resultMatrix = outSlice
	server.dim = int32(cols)
	return nil
}

/*
Done returns calculated results.
*/
func (server *CovarianceMatrixServer) Done(ctx context.Context, call CovarianceMatrix_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.covariance_matrix.Done] failed to allocate done results",
			err,
		))
	}

	listBuilder, err := results.NewCov(int32(len(server.resultMatrix)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate result matrix list", err))
	}

	for index, val := range server.resultMatrix {
		listBuilder.Set(index, val)
	}

	results.SetDim(server.dim)
	return nil
}
