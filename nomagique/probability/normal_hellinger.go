package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NormalHellingerServer calculates Hellinger distance between two Normal distributions.
*/
type NormalHellingerServer struct {
	*runtime.System
	hellinger float64
}

func NewNormalHellinger(ctx context.Context) *NormalHellingerServer {
	server := &NormalHellingerServer{
		System: runtime.NewSystem(ctx, "probability.normal_hellinger"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *NormalHellingerServer) Write(ctx context.Context, call NormalHellinger_write) error {
	args := call.Args()
	l := distuv.Normal{Mu: args.MuL(), Sigma: args.SigmaL()}
	r := distuv.Normal{Mu: args.MuR(), Sigma: args.SigmaR()}
	server.hellinger = distuv.Hellinger{}.DistNormal(l, r)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *NormalHellingerServer) Done(ctx context.Context, call NormalHellinger_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.normal_hellinger.Done] failed to allocate done results",
			err,
		))
	}

	results.SetHellinger(server.hellinger)
	return nil
}
