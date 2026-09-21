package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type BuyIntensityServer struct {
	out float64
}

func NewBuyIntensity() *BuyIntensityServer {
	return &BuyIntensityServer{}
}

func (server *BuyIntensityServer) Write(ctx context.Context, call BuyIntensity_write) error {
	server.out = call.Args().Value()
	return nil
}

func (server *BuyIntensityServer) Done(ctx context.Context, call BuyIntensity_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"buy_intensity: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
