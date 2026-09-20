package hawkes

import (
	"context"
	"time"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/core"
)

type ProcessServer struct {
	state *path
	Downstream func(context.Context, *Reading) error
}

func NewProcessServer() *ProcessServer {
	return &ProcessServer{
		state: &path{samples: make([]sample, 0)},
	}
}

func (s *ProcessServer) Write(ctx context.Context, call Process_write) error {
	args, err := call.Args().Process()
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
	
	var event [2]float64
	if payloadPtr.IsValid() {
		data := payloadPtr.Data()
		if len(data) > 0 {
			_ = sonic.Unmarshal(data, &event)
		}
	}

	atNano := int64(event[0])
	markRaw := event[1]

	if atNano <= 0 {
		return nil
	}

	at := time.Unix(0, atNano)

	if s.state.hasLast && at.Before(s.state.lastAt) {
		return nil
	}

	mark := -core.Unit
	if markRaw > 0 {
		mark = core.Unit
	}

	buyArrivals, sellArrivals := s.state.sides()
	countBuy := float64(len(buyArrivals))
	countSell := float64(len(sellArrivals))

	if mark > 0 {
		countBuy++
	}
	if mark <= 0 {
		countSell++
	}

	count := countBuy + countSell
	reading := Reading{
		EventCount:   count,
		BuyCount:     countBuy,
		SellCount:    countSell,
		BuyFraction:  countBuy / count,
		SellFraction: countSell / count,
	}

	from := at
	if len(s.state.samples) > 0 {
		from = s.state.origin()
	}

	atSec := float64(atNano) * 1e-9
	fromSec := float64(from.UnixNano()) * 1e-9
	span := atSec - fromSec

	if span > 0 {
		reading.HasRates = true
		reading.BuyRate = countBuy / span
		reading.SellRate = countSell / span
		reading.ArrivalRate = count / span
	}

	if s.state.modelReady {
		reading.HasFit = true
		evaluateReading(&reading, s.state, buyArrivals, sellArrivals, atSec)
	}

	s.state.lastAt = at
	s.state.hasLast = true
	s.state.remember(at, atSec, mark)
	s.state.refit(atSec)

	return s.Downstream(ctx, &reading)
}

func (s *ProcessServer) Done(ctx context.Context, call Process_done) error {
	return nil
}
