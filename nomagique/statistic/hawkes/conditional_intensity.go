package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type ConditionalIntensityServer struct {
	out float64
}

func NewConditionalIntensity() *ConditionalIntensityServer {
	return &ConditionalIntensityServer{}
}

func (server *ConditionalIntensityServer) Write(ctx context.Context, call ConditionalIntensity_write) error {
	server.out = call.Args().In()
	return nil
}

func (server *ConditionalIntensityServer) Done(ctx context.Context, call ConditionalIntensity_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"conditional_intensity: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
