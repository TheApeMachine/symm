package hawkes

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
)

type ProcessServer struct {
	state   *path
	reading Reading
}

func NewProcess() *ProcessServer {
	return &ProcessServer{
		state: &path{samples: make([]sample, 0)},
	}
}

func (s *ProcessServer) Write(ctx context.Context, call Process_write) error {
	args := call.Args()
	atNano := int64(args.Time())
	markRaw := args.Mark()

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

	s.reading = reading
	return nil
}

func (s *ProcessServer) Done(ctx context.Context, call Process_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetEventCount(s.reading.EventCount)
	results.SetBuyCount(s.reading.BuyCount)
	results.SetSellCount(s.reading.SellCount)
	results.SetBuyFraction(s.reading.BuyFraction)
	results.SetSellFraction(s.reading.SellFraction)
	results.SetArrivalRate(s.reading.ArrivalRate)
	results.SetBuyRate(s.reading.BuyRate)
	results.SetSellRate(s.reading.SellRate)
	results.SetLambda(s.reading.Lambda)
	results.SetLambdaBuy(s.reading.LambdaBuy)
	results.SetLambdaSell(s.reading.LambdaSell)
	results.SetSpectralRadius(s.reading.SpectralRadius)

	s.reading = Reading{}
	return nil
}
