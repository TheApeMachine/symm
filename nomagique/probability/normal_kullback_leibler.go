package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NormalKullbackLeiblerServer calculates Kullback-Leibler divergence between two Normal distributions.
*/
type NormalKullbackLeiblerServer struct {
	*runtime.System
	kl float64
}

func NewNormalKullbackLeibler(ctx context.Context) *NormalKullbackLeiblerServer {
	server := &NormalKullbackLeiblerServer{
		System: runtime.NewSystem(ctx, "probability.normal_kullback_leibler"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *NormalKullbackLeiblerServer) Write(ctx context.Context, call NormalKullbackLeibler_write) error {
	args := call.Args()
	l := distuv.Normal{Mu: args.MuL(), Sigma: args.SigmaL()}
	r := distuv.Normal{Mu: args.MuR(), Sigma: args.SigmaR()}
	server.kl = distuv.KullbackLeibler{}.DistNormal(l, r)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *NormalKullbackLeiblerServer) Done(ctx context.Context, call NormalKullbackLeibler_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.normal_kullback_leibler.Done] failed to allocate done results",
			err,
		))
	}

	results.SetKl(server.kl)
	return nil
}
