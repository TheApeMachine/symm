package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ChiSquareServer calculates Chi-Square goodness-of-fit test statistic.
*/
type ChiSquareServer struct {
	*runtime.System
	result float64
}

func NewChiSquare(ctx context.Context) *ChiSquareServer {
	server := &ChiSquareServer{
		System: runtime.NewSystem(ctx, "statistic.chi_square"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *ChiSquareServer) Write(ctx context.Context, call ChiSquare_write) error {
	obsList, err := call.Args().Observed()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read observed", err))
	}

	expList, err := call.Args().Expected()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read expected", err))
	}

	length := obsList.Len()
	sliceObs := make([]float64, length)
	sliceExp := make([]float64, length)

	for index := 0; index < length; index++ {
		sliceObs[index] = obsList.At(index)
		sliceExp[index] = expList.At(index)
	}

	server.result = stat.ChiSquare(sliceObs, sliceExp)
	return nil
}

/*
Done returns calculated results.
*/
func (server *ChiSquareServer) Done(ctx context.Context, call ChiSquare_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.chi_square.Done] failed to allocate done results",
			err,
		))
	}

	results.SetChiSquare(server.result)
	return nil
}
