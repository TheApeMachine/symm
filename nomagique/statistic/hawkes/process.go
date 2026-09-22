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

	// The fitted kernel travels with the intensities it produced: how strongly
	// each direction excites each other, how fast that excitation decays, and
	// how many further arrivals one arrival is expected to set off. Without it
	// a burst and a steady rate of the same average are indistinguishable.
	results.SetMuBuy(s.reading.MuBuy)
	results.SetMuSell(s.reading.MuSell)
	results.SetMu(s.reading.Mu)
	results.SetExcessBuy(s.reading.ExcessBuy)
	results.SetExcessSell(s.reading.ExcessSell)
	results.SetExcitationBuy(s.reading.ExcitationBuyFrac)
	results.SetExcitationSell(s.reading.ExcitationSellFrac)
	results.SetAlphaBuyBuy(s.reading.AlphaBB)
	results.SetAlphaBuySell(s.reading.AlphaBS)
	results.SetAlphaSellBuy(s.reading.AlphaSB)
	results.SetAlphaSellSell(s.reading.AlphaSS)
	results.SetBeta(s.reading.Beta)
	results.SetTimescale(s.reading.Timescale)
	results.SetOffspringBuyBuy(s.reading.OffspringBB)
	results.SetOffspringBuySell(s.reading.OffspringBS)
	results.SetOffspringSellBuy(s.reading.OffspringSB)
	results.SetOffspringSellSell(s.reading.OffspringSS)
	results.SetDescendantsBuy(s.reading.DescendantsBuy)
	results.SetDescendantsSell(s.reading.DescendantsSell)
	results.SetCompensatorBuy(s.reading.CompensatorBuy)
	results.SetCompensatorSell(s.reading.CompensatorSell)
	results.SetInnovationBuy(s.reading.InnovationBuy)
	results.SetInnovationSell(s.reading.InnovationSell)
	results.SetSnr(s.reading.SNR)
	results.SetLogLikelihoodHawkes(s.reading.HawkesLogLikelihood)
	results.SetLogLikelihoodPoisson(s.reading.PoissonLogLikelihood)
	results.SetLogLikelihoodSelfOnly(s.reading.SelfOnlyLogLikelihood)

	s.reading = Reading{}
	return nil
}
