package hawkes

import (
	"context"
)

type SpectralRadiusServer struct {
	Downstream func(context.Context, float64) error
}

func NewSpectralRadiusServer() *SpectralRadiusServer {
	return &SpectralRadiusServer{}
}

func (s *SpectralRadiusServer) Write(ctx context.Context, call SpectralRadius_write) error {
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
	result := reading.SpectralRadius
	return s.Downstream(ctx, result)
}

func (s *SpectralRadiusServer) Done(ctx context.Context, call SpectralRadius_done) error {
	return nil
}
