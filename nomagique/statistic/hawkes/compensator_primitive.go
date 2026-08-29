package hawkes

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Compensator-scoped Frame facts, per signal/hawkes/README.md section 19.
*/
var (
	SymbolCompensatorBuy    = types.MustIntern("hawkes/state/compensator_buy")
	SymbolCompensatorSell   = types.MustIntern("hawkes/state/compensator_sell")
	SymbolInnovationBuy     = types.MustIntern("hawkes/obs/count_innovation_buy")
	SymbolInnovationSell    = types.MustIntern("hawkes/obs/count_innovation_sell")
	SymbolStandardInnovBuy  = types.MustIntern("hawkes/obs/standardized_innovation_buy")
	SymbolStandardInnovSell = types.MustIntern("hawkes/obs/standardized_innovation_sell")
	SymbolSNR               = types.MustIntern("hawkes/obs/snr")
)

/*
Compensator integrates the fitted per-side pre-arrival conditional intensity
over the interval opened by the immediately preceding retained event, adding
that increment to the running Λ_x = ∫λ_x(t)dt since the first retained
observation. It requires a converged fit — the same absence-over-fallback
rule as ConditionalIntensity and Likelihood applies here.
*/
func Compensator(input *types.Frame) {
	muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta, ok := ReadModel(input)

	if !ok {
		return
	}

	buy, sell, ok := retainedArrivals(input)

	if !ok {
		buy, sell = nil, nil
	}

	stream := newArrivalStream(buy, sell)
	horizonSec := eventHorizonSec(input)
	lastEventSec := stream.originSec

	if len(stream.marked) > 0 {
		lastEventSec = stream.marked[len(stream.marked)-1].atSec
	}

	span := horizonSec - lastEventSec

	if span <= 0 {
		return
	}

	buySupport := observationKernelIntegralSupport(stream.buy, lastEventSec, horizonSec, beta)
	sellSupport := observationKernelIntegralSupport(stream.sell, lastEventSec, horizonSec, beta)

	buyIncrement := muX*span + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
	sellIncrement := muY*span + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport

	if !finite(buyIncrement, sellIncrement) {
		return
	}

	compensatorBuy, _ := input.Get(SymbolCompensatorBuy)
	compensatorSell, _ := input.Get(SymbolCompensatorSell)
	compensatorBuy += buyIncrement
	compensatorSell += sellIncrement

	input.Put(SymbolCompensatorBuy, compensatorBuy)
	input.Put(SymbolCompensatorSell, compensatorSell)

	countBuy := float64(len(buy))
	countSell := float64(len(sell))
	innovationBuy := countBuy - compensatorBuy
	innovationSell := countSell - compensatorSell

	input.Put(SymbolInnovationBuy, innovationBuy)
	input.Put(SymbolInnovationSell, innovationSell)

	if compensatorBuy > 0 {
		input.Put(SymbolStandardInnovBuy, innovationBuy/math.Sqrt(compensatorBuy))
	}

	if compensatorSell > 0 {
		input.Put(SymbolStandardInnovSell, innovationSell/math.Sqrt(compensatorSell))
	}

	putSNR(input, muX, muY, compensatorBuy, compensatorSell)
}

/*
putSNR reports the joint SNR from README section 20:
SNR = (1/k) * sum_x (E_x^2 / Lambda_x), where E_x = Lambda_x - mu_x*T over the
full retained observation span [From, At], and k is the count of sides whose
compensator is defined (positive). SNR is left absent when neither side has a
positive compensator, since the decomposition it reports on is itself
undefined in that case.
*/
func putSNR(input *types.Frame, muX, muY, compensatorBuy, compensatorSell float64) {
	fromSec, hasFrom := input.Get(SymbolFromSec)
	atSec, hasAt := input.Get(SymbolAtSec)

	if !hasFrom || !hasAt {
		return
	}

	span := atSec - fromSec

	if span <= 0 {
		return
	}

	sum := 0.0
	sides := 0

	if compensatorBuy > 0 {
		excess := compensatorBuy - muX*span
		sum += (excess * excess) / compensatorBuy
		sides++
	}

	if compensatorSell > 0 {
		excess := compensatorSell - muY*span
		sum += (excess * excess) / compensatorSell
		sides++
	}

	if sides == 0 {
		return
	}

	input.Put(SymbolSNR, sum/float64(sides))
}
