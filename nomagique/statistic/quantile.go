package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
QuantileServer calculates the p-th quantile of values.
*/
type QuantileServer struct {
	*runtime.System
	result float64
}

func NewQuantile(ctx context.Context) *QuantileServer {
	server := &QuantileServer{
		System: runtime.NewSystem(ctx, "statistic.quantile"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *QuantileServer) Write(ctx context.Context, call Quantile_write) error {
	pVal := call.Args().P()
	valsList, err := call.Args().Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read values", err))
	}

	length := valsList.Len()

	if length == 0 {
		return nil
	}

	slice := make([]float64, length)

	for index := 0; index < length; index++ {
		slice[index] = valsList.At(index)
	}

	stat.SortWeighted(slice, nil)
	server.result = stat.Quantile(pVal, stat.Empirical, slice, nil)
	return nil
}

/*
Done returns calculated results.
*/
func (server *QuantileServer) Done(ctx context.Context, call Quantile_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.quantile.Done] failed to allocate done results",
			err,
		))
	}

	results.SetQuantile(server.result)
	return nil
}
