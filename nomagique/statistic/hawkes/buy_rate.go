package hawkes

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type BuyRateServer struct {
	Downstream types.Float64Sink
}

func NewBuyRate() *BuyRateServer {
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

	reading, err := extractReading(payloadPtr)
	if err != nil || reading == nil {
		return nil
	}

	result := reading.BuyRate
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *BuyRateServer) Done(ctx context.Context, call BuyRate_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}
