package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BetaHellingerServer calculates Hellinger distance between two Beta distributions.
*/
type BetaHellingerServer struct {
	*runtime.System
	hellinger float64
}

func NewBetaHellinger(ctx context.Context) *BetaHellingerServer {
	server := &BetaHellingerServer{
		System: runtime.NewSystem(ctx, "probability.beta_hellinger"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *BetaHellingerServer) Write(ctx context.Context, call BetaHellinger_write) error {
	args := call.Args()
	l := distuv.Beta{Alpha: args.AlphaL(), Beta: args.BetaL()}
	r := distuv.Beta{Alpha: args.AlphaR(), Beta: args.BetaR()}
	server.hellinger = distuv.Hellinger{}.DistBeta(l, r)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *BetaHellingerServer) Done(ctx context.Context, call BetaHellinger_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.beta_hellinger.Done] failed to allocate done results",
			err,
		))
	}

	results.SetHellinger(server.hellinger)
	return nil
}
