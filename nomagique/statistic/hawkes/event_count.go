package hawkes

import (
	"context"
)

type EventCountServer struct {
	Downstream func(context.Context, float64) error
}

func NewEventCountServer() *EventCountServer {
	return &EventCountServer{}
}

func (s *EventCountServer) Write(ctx context.Context, call EventCount_write) error {
	args, err := call.Args().View()
	if err != nil {
		return err
	}
	
	payloadPtr, err := args.Payload()
	if err != nil {
		return err
	}

	if s.Downstream == nil {
		return nil
	}
	
	reading, err := extractReading(payloadPtr)
	if err != nil || reading == nil {
		return nil
	}
	result := reading.EventCount
	return s.Downstream(ctx, result)
}

func (s *EventCountServer) Done(ctx context.Context, call EventCount_done) error {
	return nil
}
