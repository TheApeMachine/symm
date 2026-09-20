package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type ArrivalRateServer struct {
	out float64
}

func NewArrivalRate() *ArrivalRateServer {
	return &ArrivalRateServer{}
}

func (server *ArrivalRateServer) Write(ctx context.Context, call ArrivalRate_write) error {
	server.out = call.Args().In()
	return nil
}

func (server *ArrivalRateServer) Done(ctx context.Context, call ArrivalRate_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"arrival_rate: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
