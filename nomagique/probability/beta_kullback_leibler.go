package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BetaKullbackLeiblerServer calculates Kullback-Leibler divergence between two Beta distributions.
*/
type BetaKullbackLeiblerServer struct {
	*runtime.System
	kl float64
}

func NewBetaKullbackLeibler(ctx context.Context) *BetaKullbackLeiblerServer {
	server := &BetaKullbackLeiblerServer{
		System: runtime.NewSystem(ctx, "probability.beta_kullback_leibler"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *BetaKullbackLeiblerServer) Write(ctx context.Context, call BetaKullbackLeibler_write) error {
	args := call.Args()
	l := distuv.Beta{Alpha: args.AlphaL(), Beta: args.BetaL()}
	r := distuv.Beta{Alpha: args.AlphaR(), Beta: args.BetaR()}
	server.kl = distuv.KullbackLeibler{}.DistBeta(l, r)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *BetaKullbackLeiblerServer) Done(ctx context.Context, call BetaKullbackLeibler_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.beta_kullback_leibler.Done] failed to allocate done results",
			err,
		))
	}

	results.SetKl(server.kl)
	return nil
}
