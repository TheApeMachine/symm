package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type SellIntensityServer struct {
	out float64
}

func NewSellIntensity() *SellIntensityServer {
	return &SellIntensityServer{}
}

func (server *SellIntensityServer) Write(ctx context.Context, call SellIntensity_write) error {
	server.out = call.Args().Value()
	return nil
}

func (server *SellIntensityServer) Done(ctx context.Context, call SellIntensity_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"sell_intensity: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
