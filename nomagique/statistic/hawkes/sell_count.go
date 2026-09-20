package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type SellCountServer struct {
	out float64
}

func NewSellCount() *SellCountServer {
	return &SellCountServer{}
}

func (server *SellCountServer) Write(ctx context.Context, call SellCount_write) error {
	server.out = call.Args().In()
	return nil
}

func (server *SellCountServer) Done(ctx context.Context, call SellCount_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"sell_count: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
