package hawkes

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type BuyRateServer struct {
	Downstream func(context.Context, float64) error
}

func NewBuyRateServer() *BuyRateServer {
	return &BuyRateServer{}
}

func (s *BuyRateServer) Write(ctx context.Context, call BuyRate_write) error {
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
	result := reading.BuyRate
	return s.Downstream(ctx, result)
}

func (s *BuyRateServer) Done(ctx context.Context, call BuyRate_done) error {
	return nil
}



type BuyRateNode types.StreamNode[any, any]

func NewBuyRate() BuyRateNode {
	server := &BuyRateServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
