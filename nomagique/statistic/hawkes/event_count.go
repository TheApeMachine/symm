package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
)

type EventCountServer struct {
	out float64
}

func NewEventCount() *EventCountServer {
	return &EventCountServer{}
}

func (server *EventCountServer) Write(ctx context.Context, call EventCount_write) error {
	server.out = call.Args().Value()
	return nil
}

func (server *EventCountServer) Done(ctx context.Context, call EventCount_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"event_count: alloc results failed",
			err,
		))
	}

	result.SetOut(server.out)
	server.out = 0
	return nil
}
