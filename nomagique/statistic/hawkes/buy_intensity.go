package hawkes

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type BuyIntensityServer struct {
	Downstream func(context.Context, float64) error
}

func NewBuyIntensityServer() *BuyIntensityServer {
	return &BuyIntensityServer{}
}

func (s *BuyIntensityServer) Write(ctx context.Context, call BuyIntensity_write) error {
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
	result := reading.LambdaBuy
	return s.Downstream(ctx, result)
}

func (s *BuyIntensityServer) Done(ctx context.Context, call BuyIntensity_done) error {
	return nil
}



type BuyIntensityNode types.StreamNode[any, any]

func NewBuyIntensity() BuyIntensityNode {
	server := &BuyIntensityServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
