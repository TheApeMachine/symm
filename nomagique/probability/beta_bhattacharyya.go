package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BetaBhattacharyyaServer calculates Bhattacharyya distance between two Beta distributions.
*/
type BetaBhattacharyyaServer struct {
	*runtime.System
	bhattacharyya float64
}

func NewBetaBhattacharyya(ctx context.Context) *BetaBhattacharyyaServer {
	server := &BetaBhattacharyyaServer{
		System: runtime.NewSystem(ctx, "probability.beta_bhattacharyya"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *BetaBhattacharyyaServer) Write(ctx context.Context, call BetaBhattacharyya_write) error {
	args := call.Args()
	l := distuv.Beta{Alpha: args.AlphaL(), Beta: args.BetaL()}
	r := distuv.Beta{Alpha: args.AlphaR(), Beta: args.BetaR()}
	server.bhattacharyya = distuv.Bhattacharyya{}.DistBeta(l, r)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *BetaBhattacharyyaServer) Done(ctx context.Context, call BetaBhattacharyya_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.beta_bhattacharyya.Done] failed to allocate done results",
			err,
		))
	}

	results.SetBhattacharyya(server.bhattacharyya)
	return nil
}
