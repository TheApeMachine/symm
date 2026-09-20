package learning

import (
	"context"

	"github.com/theapemachine/errnie"
)

type CounterfactualServer struct {
	effect float64
	out    float64
}

func NewCounterfactual() *CounterfactualServer {
	return &CounterfactualServer{}
}

func (server *CounterfactualServer) Write(ctx context.Context, call Counterfactual_write) error {
	args := call.Args()
	treatment := args.Treatment()
	outcome := args.Outcome()
	confounder := args.Confounder()

	server.effect = outcome - treatment*confounder
	server.out = server.effect
	return nil
}

func (server *CounterfactualServer) Done(ctx context.Context, call Counterfactual_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"counterfactual: alloc results failed",
			err,
		))
	}

	results.SetEffect(server.effect)
	results.SetOut(server.out)
	server.effect = 0
	server.out = 0
	return nil
}
