package learning

import (
	"context"

	"github.com/theapemachine/errnie"
)

type LinearPredictionServer struct {
	out float64
}

func NewLinearPrediction() *LinearPredictionServer {
	return &LinearPredictionServer{}
}

func (server *LinearPredictionServer) Write(ctx context.Context, call LinearPrediction_write) error {
	args := call.Args()
	server.out = args.Slope()*args.X() + args.Intercept()
	return nil
}

func (server *LinearPredictionServer) Done(ctx context.Context, call LinearPrediction_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"linear_prediction: alloc results failed",
			err,
		))
	}

	results.SetOut(server.out)
	server.out = 0
	return nil
}
