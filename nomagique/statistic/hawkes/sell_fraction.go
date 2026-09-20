package hawkes

import (
	"context"
)

type SellFractionServer struct {
	Downstream func(context.Context, float64) error
}

func NewSellFractionServer() *SellFractionServer {
	return &SellFractionServer{}
}

func (s *SellFractionServer) Write(ctx context.Context, call SellFraction_write) error {
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
	result := reading.SellFraction
	return s.Downstream(ctx, result)
}

func (s *SellFractionServer) Done(ctx context.Context, call SellFraction_done) error {
	return nil
}
