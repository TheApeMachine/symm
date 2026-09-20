package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type BuyCountServer struct {
	out float64
}

func NewBuyCount() *BuyCountServer {
	return &BuyCountServer{}
}

func (server *BuyCountServer) Write(ctx context.Context, call BuyCount_write) error {
	server.out = call.Args().In()
	return nil
}

func (server *BuyCountServer) Done(ctx context.Context, call BuyCount_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"buy_count: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
