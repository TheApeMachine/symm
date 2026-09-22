package statistic

import (
	"context"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MahalanobisServer calculates Mahalanobis distance between two vectors with Cholesky factor.
*/
type MahalanobisServer struct {
	*runtime.System
	result float64
}

func NewMahalanobis(ctx context.Context) *MahalanobisServer {
	server := &MahalanobisServer{
		System: runtime.NewSystem(ctx, "statistic.mahalanobis"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MahalanobisServer) Write(ctx context.Context, call Mahalanobis_write) error {
	xList, err := call.Args().X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read x", err))
	}

	yList, err := call.Args().Y()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read y", err))
	}

	cholList, err := call.Args().CholData()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read chol data", err))
	}

	dim := int(call.Args().Dim())
	vecXSlice := make([]float64, dim)
	vecYSlice := make([]float64, dim)
	symSlice := make([]float64, dim*dim)

	for index := 0; index < dim; index++ {
		vecXSlice[index] = xList.At(index)
		vecYSlice[index] = yList.At(index)
	}

	for index := 0; index < dim*dim; index++ {
		symSlice[index] = cholList.At(index)
	}

	denseX := mat.NewVecDense(dim, vecXSlice)
	denseY := mat.NewVecDense(dim, vecYSlice)
	symMat := mat.NewSymDense(dim, nil)

	for r := 0; r < dim; r++ {
		for c := r; c < dim; c++ {
			symMat.SetSym(r, c, symSlice[r*dim+c])
		}
	}

	var chol mat.Cholesky
	ok := chol.Factorize(symMat)

	if !ok {
		return errnie.Error(errnie.Err(errnie.Internal, "cholesky factorize failed in mahalanobis", nil))
	}

	server.result = stat.Mahalanobis(denseX, denseY, &chol)
	return nil
}

/*
Done returns calculated results.
*/
func (server *MahalanobisServer) Done(ctx context.Context, call Mahalanobis_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.mahalanobis.Done] failed to allocate done results",
			err,
		))
	}

	results.SetDistance(server.result)
	return nil
}
