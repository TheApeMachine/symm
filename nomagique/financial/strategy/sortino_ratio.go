package strategy

import (
	"context"
	"math"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SortinoRatioServer calculates the online annualized Sortino Ratio of cumulative outcomes.
*/
type SortinoRatioServer struct {
	*runtime.System
	prevEquity float64
	mean float64
	downsideSumSq float64
	downsideCount int
	returnsCount int
	count int
	ratio float64
}

func NewSortinoRatio(ctx context.Context) *SortinoRatioServer {
	server := &SortinoRatioServer{
		System: runtime.NewSystem(ctx, "financial.strategy.sortino_ratio"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *SortinoRatioServer) Write(ctx context.Context, call SortinoRatio_write) error {
	outcomeVal := call.Args().Outcome()

	currEquity := 1.0 + outcomeVal

	if server.count > 0 && server.prevEquity > 0 {
		periodReturn := (currEquity - server.prevEquity) / server.prevEquity
		server.returnsCount++
		delta := periodReturn - server.mean
		server.mean += delta / float64(server.returnsCount)

		if periodReturn < 0 {
			server.downsideSumSq += periodReturn * periodReturn
			server.downsideCount++
		}

		if server.downsideCount > 0 {
			downsideDev := math.Sqrt(server.downsideSumSq / float64(server.returnsCount))

			if downsideDev >= 1e-9 {
				server.ratio = (server.mean / downsideDev) * math.Sqrt(252.0)
			}
		}
	}

	server.prevEquity = currEquity
	server.count++
	return nil
}

/*
Done returns calculated strategy results.
*/
func (server *SortinoRatioServer) Done(ctx context.Context, call SortinoRatio_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.sortino_ratio.Done] failed to allocate done results",
			err,
		))
	}

	results.SetRatio(server.ratio)
	return nil
}
