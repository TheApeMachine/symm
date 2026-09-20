package hawkes

import (
	"context"
)

type ArrivalRateServer struct {
	Downstream func(context.Context, float64) error
}

func NewArrivalRateServer() *ArrivalRateServer {
	return &ArrivalRateServer{}
}

func (s *ArrivalRateServer) Write(ctx context.Context, call ArrivalRate_write) error {
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
	result := reading.ArrivalRate
	return s.Downstream(ctx, result)
}

func (s *ArrivalRateServer) Done(ctx context.Context, call ArrivalRate_done) error {
	return nil
}
