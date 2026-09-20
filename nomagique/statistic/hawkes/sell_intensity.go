package hawkes

import (
	"context"
)

type SellIntensityServer struct {
	Downstream func(context.Context, float64) error
}

func NewSellIntensityServer() *SellIntensityServer {
	return &SellIntensityServer{}
}

func (s *SellIntensityServer) Write(ctx context.Context, call SellIntensity_write) error {
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
	result := reading.LambdaSell
	return s.Downstream(ctx, result)
}

func (s *SellIntensityServer) Done(ctx context.Context, call SellIntensity_done) error {
	return nil
}
