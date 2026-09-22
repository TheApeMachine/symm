package strategy

import (
	"context"
	"math"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
SharpeRatioServer calculates the online annualized Sharpe Ratio of cumulative outcomes.
*/
type SharpeRatioServer struct {
	*runtime.System
	prevEquity float64
	mean float64
	m2 float64
	returnsCount int
	count int
	ratio float64
}

func NewSharpeRatio(ctx context.Context) *SharpeRatioServer {
	server := &SharpeRatioServer{
		System: runtime.NewSystem(ctx, "financial.strategy.sharpe_ratio"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *SharpeRatioServer) Write(ctx context.Context, call SharpeRatio_write) error {
	outcomeVal := call.Args().Outcome()

	currEquity := 1.0 + outcomeVal

	if server.count > 0 && server.prevEquity > 0 {
		periodReturn := (currEquity - server.prevEquity) / server.prevEquity
		server.returnsCount++
		delta := periodReturn - server.mean
		server.mean += delta / float64(server.returnsCount)
		delta2 := periodReturn - server.mean
		server.m2 += delta * delta2

		if server.returnsCount > 1 {
			variance := server.m2 / float64(server.returnsCount-1)
			stdDev := math.Sqrt(variance)

			if stdDev >= 1e-9 {
				server.ratio = (server.mean / stdDev) * math.Sqrt(252.0)
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
func (server *SharpeRatioServer) Done(ctx context.Context, call SharpeRatio_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.sharpe_ratio.Done] failed to allocate done results",
			err,
		))
	}

	results.SetRatio(server.ratio)
	return nil
}
