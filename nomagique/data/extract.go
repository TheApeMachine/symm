package data

import (
	"context"
)

type ExtractServer struct {
	Downstream func(context.Context, float64) error
}

func NewExtract() *ExtractServer {
	return &ExtractServer{}
}

func (s *ExtractServer) Evaluate(ctx context.Context, payload any) (float64, error) {
	val := 0.0
	if v, ok := payload.(float64); ok {
		val = v
	}

	if s.Downstream != nil {
		return val, s.Downstream(ctx, val)
	}

	return val, nil
}

func (s *ExtractServer) Write(ctx context.Context, call Extract_write) error {
	args, err := call.Args().Extract()
	if err != nil {
		return err
	}

	payloadPtr, err := args.Payload()
	if err != nil {
		return err
	}

	var payload any
	if payloadPtr.IsValid() {
		// placeholder
	}

	_, evalErr := s.Evaluate(ctx, payload)
	return evalErr
}

func (s *ExtractServer) Done(ctx context.Context, call Extract_done) error {
	return nil
}
