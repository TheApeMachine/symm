package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type SellRateServer struct {
	out float64
}

func NewSellRate() *SellRateServer {
	return &SellRateServer{}
}

func (server *SellRateServer) Write(ctx context.Context, call SellRate_write) error {
	server.out = call.Args().Value()
	return nil
}

func (server *SellRateServer) Done(ctx context.Context, call SellRate_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"sell_rate: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
