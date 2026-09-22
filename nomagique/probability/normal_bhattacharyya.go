package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NormalBhattacharyyaServer calculates Bhattacharyya distance between two Normal distributions.
*/
type NormalBhattacharyyaServer struct {
	*runtime.System
	bhattacharyya float64
}

func NewNormalBhattacharyya(ctx context.Context) *NormalBhattacharyyaServer {
	server := &NormalBhattacharyyaServer{
		System: runtime.NewSystem(ctx, "probability.normal_bhattacharyya"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *NormalBhattacharyyaServer) Write(ctx context.Context, call NormalBhattacharyya_write) error {
	args := call.Args()
	l := distuv.Normal{Mu: args.MuL(), Sigma: args.SigmaL()}
	r := distuv.Normal{Mu: args.MuR(), Sigma: args.SigmaR()}
	server.bhattacharyya = distuv.Bhattacharyya{}.DistNormal(l, r)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *NormalBhattacharyyaServer) Done(ctx context.Context, call NormalBhattacharyya_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.normal_bhattacharyya.Done] failed to allocate done results",
			err,
		))
	}

	results.SetBhattacharyya(server.bhattacharyya)
	return nil
}
