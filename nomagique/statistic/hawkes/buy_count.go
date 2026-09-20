package hawkes

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type BuyCountServer struct {
	Downstream func(context.Context, float64) error
}

func NewBuyCountServer() *BuyCountServer {
	return &BuyCountServer{}
}

func (s *BuyCountServer) Write(ctx context.Context, call BuyCount_write) error {
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
	result := reading.BuyCount
	return s.Downstream(ctx, result)
}

func (s *BuyCountServer) Done(ctx context.Context, call BuyCount_done) error {
	return nil
}



type BuyCountNode types.StreamNode[any, any]

func NewBuyCount() BuyCountNode {
	server := &BuyCountServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
