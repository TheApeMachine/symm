package hawkes

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type ConditionalIntensityServer struct {
	Downstream func(context.Context, float64) error
}

func NewConditionalIntensityServer() *ConditionalIntensityServer {
	return &ConditionalIntensityServer{}
}

func (s *ConditionalIntensityServer) Write(ctx context.Context, call ConditionalIntensity_write) error {
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
	result := reading.Lambda
	return s.Downstream(ctx, result)
}

func (s *ConditionalIntensityServer) Done(ctx context.Context, call ConditionalIntensity_done) error {
	return nil
}



type ConditionalIntensityNode types.StreamNode[any, any]

func NewConditionalIntensity() ConditionalIntensityNode {
	server := &ConditionalIntensityServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
