package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type BuyRateServer struct {
	out float64
}

func NewBuyRate() *BuyRateServer {
	return &BuyRateServer{}
}

func (server *BuyRateServer) Write(ctx context.Context, call BuyRate_write) error {
	server.out = call.Args().In()
	return nil
}

func (server *BuyRateServer) Done(ctx context.Context, call BuyRate_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"buy_rate: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
