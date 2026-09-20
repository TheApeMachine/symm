package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type SellFractionServer struct {
	out float64
}

func NewSellFraction() *SellFractionServer {
	return &SellFractionServer{}
}

func (server *SellFractionServer) Write(ctx context.Context, call SellFraction_write) error {
	server.out = call.Args().In()
	return nil
}

func (server *SellFractionServer) Done(ctx context.Context, call SellFraction_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"sell_fraction: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
