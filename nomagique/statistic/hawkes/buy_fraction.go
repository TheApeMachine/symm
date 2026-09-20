package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type BuyFractionServer struct {
	out float64
}

func NewBuyFraction() *BuyFractionServer {
	return &BuyFractionServer{}
}

func (server *BuyFractionServer) Write(ctx context.Context, call BuyFraction_write) error {
	server.out = call.Args().In()
	return nil
}

func (server *BuyFractionServer) Done(ctx context.Context, call BuyFraction_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"buy_fraction: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
