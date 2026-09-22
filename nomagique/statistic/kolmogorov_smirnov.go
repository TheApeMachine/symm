package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
KolmogorovSmirnovServer calculates two-sample Kolmogorov-Smirnov test statistic.
*/
type KolmogorovSmirnovServer struct {
	*runtime.System
	result float64
}

func NewKolmogorovSmirnov(ctx context.Context) *KolmogorovSmirnovServer {
	server := &KolmogorovSmirnovServer{
		System: runtime.NewSystem(ctx, "statistic.kolmogorov_smirnov"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *KolmogorovSmirnovServer) Write(ctx context.Context, call KolmogorovSmirnov_write) error {
	xList, err := call.Args().X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read x", err))
	}

	yList, err := call.Args().Y()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read y", err))
	}

	lenX := xList.Len()
	lenY := yList.Len()
	sliceX := make([]float64, lenX)
	sliceY := make([]float64, lenY)

	for index := 0; index < lenX; index++ {
		sliceX[index] = xList.At(index)
	}

	for index := 0; index < lenY; index++ {
		sliceY[index] = yList.At(index)
	}

	server.result = stat.KolmogorovSmirnov(sliceX, nil, sliceY, nil)
	return nil
}

/*
Done returns calculated results.
*/
func (server *KolmogorovSmirnovServer) Done(ctx context.Context, call KolmogorovSmirnov_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.kolmogorov_smirnov.Done] failed to allocate done results",
			err,
		))
	}

	results.SetStatistic(server.result)
	return nil
}
