package hawkes

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Compensator-scoped Frame facts, per signal/hawkes/README.md sections 19-20.
*/
var (
	SymbolCompensatorBuy    = types.MustIntern("hawkes/obs/compensator_buy")
	SymbolCompensatorSell   = types.MustIntern("hawkes/obs/compensator_sell")
	SymbolInnovationBuy     = types.MustIntern("hawkes/obs/count_innovation_buy")
	SymbolInnovationSell    = types.MustIntern("hawkes/obs/count_innovation_sell")
	SymbolStandardInnovBuy  = types.MustIntern("hawkes/obs/standardized_innovation_buy")
	SymbolStandardInnovSell = types.MustIntern("hawkes/obs/standardized_innovation_sell")
	SymbolSNR               = types.MustIntern("hawkes/obs/snr")
)

/*
Compensator integrates the fitted per-side conditional intensity fresh over
the full retained window [From, At] under the model fitted before this
event — Λ_x = ∫λ_x(t)dt from the first retained observation to the current
event — rather than accumulating an increment across calls: an accumulated
running total would sum intervals evaluated under DIFFERENT fitted θ
whenever a refit occurred in between, which is not the Λ_x README section 19
defines for a single fitted model over one observation interval. It requires
a converged fit — the same absence-over-fallback rule as ConditionalIntensity
and Likelihood applies here.
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
	span := stream.span(horizonSec)

	if span <= 0 {
		return
	}

	buySupport, sellSupport := stream.kernelIntegralSupport(horizonSec, beta)

	compensatorBuy := muX*span + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
	compensatorSell := muY*span + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport

	if !finite(compensatorBuy, compensatorSell) {
		return
	}

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

	putSNR(input, muX, muY, span, compensatorBuy, compensatorSell)
}

/*
putSNR reports the joint SNR from README section 20:
SNR = (1/k) * sum_x (E_x^2 / Lambda_x), where E_x = Lambda_x - mu_x*T over
the same span the compensator was just integrated over, and k is the count
of sides whose compensator is defined (positive). SNR is left absent when
neither side has a positive compensator, since the decomposition it reports
on is itself undefined in that case.
*/
func putSNR(input *types.Frame, muX, muY, span, compensatorBuy, compensatorSell float64) {
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
