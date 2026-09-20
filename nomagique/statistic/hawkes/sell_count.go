package hawkes

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type SellCountServer struct {
	Downstream func(context.Context, float64) error
}

func NewSellCountServer() *SellCountServer {
	return &SellCountServer{}
}

func (s *SellCountServer) Write(ctx context.Context, call SellCount_write) error {
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
	result := reading.SellCount
	return s.Downstream(ctx, result)
}

func (s *SellCountServer) Done(ctx context.Context, call SellCount_done) error {
	return nil
}



type SellCountNode types.StreamNode[any, any]

func NewSellCount() SellCountNode {
	server := &SellCountServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
