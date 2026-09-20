package hawkes

import (
	"context"
)

type SellRateServer struct {
	Downstream func(context.Context, float64) error
}

func NewSellRateServer() *SellRateServer {
	return &SellRateServer{}
}

func (s *SellRateServer) Write(ctx context.Context, call SellRate_write) error {
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
	result := reading.SellRate
	return s.Downstream(ctx, result)
}

func (s *SellRateServer) Done(ctx context.Context, call SellRate_done) error {
	return nil
}
