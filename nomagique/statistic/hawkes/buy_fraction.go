package hawkes

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type BuyFractionServer struct {
	Downstream func(context.Context, float64) error
}

func NewBuyFractionServer() *BuyFractionServer {
	return &BuyFractionServer{}
}

func (s *BuyFractionServer) Write(ctx context.Context, call BuyFraction_write) error {
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
	result := reading.BuyFraction
	return s.Downstream(ctx, result)
}

func (s *BuyFractionServer) Done(ctx context.Context, call BuyFraction_done) error {
	return nil
}



type BuyFractionNode types.StreamNode[any, any]

func NewBuyFraction() BuyFractionNode {
	server := &BuyFractionServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
