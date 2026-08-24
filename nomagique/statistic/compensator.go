package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolCompensatorAlpha     = types.MustIntern("compensator/alpha")
	SymbolCompensatorBeta      = types.MustIntern("compensator/beta")
	SymbolInnovationAlpha      = types.MustIntern("innovation/alpha")
	SymbolInnovationBeta       = types.MustIntern("innovation/beta")
	SymbolStandardInnovAlpha   = types.MustIntern("standardized_innovation/alpha")
	SymbolStandardInnovBeta    = types.MustIntern("standardized_innovation/beta")
	symbolCompensatorLastSec   = types.MustIntern("compensator/last_sec")
	symbolCompensatorLastNsec  = types.MustIntern("compensator/last_nsec")
)

/*
Compensator integrates the fitted per-side conditional intensity over the
event clock: Λ_x = ∫λ_x(t)dt from the first retained observation. The
increment uses the intensity that prevailed during the interval just closed —
the decayed pre-arrival intensity recovered from the Hawkes intensity state —
so the current event's own jump never inflates its own compensator. It also
emits the compensated count innovations M_x = N_x − Λ_x and their standardized
forms M_x/√Λ_x, which form a martingale under a correctly specified process.
*/
func Compensator(input types.Frame) types.Frame {
	sec, hasSec := input.Get(SymbolUnixSec)
	nsec, hasNsec := input.Get(SymbolUnixNsec)

	if !hasSec || !hasNsec || nsec < 0 || nsec >= 1e9 {
		input.Err = fmt.Errorf(
			"statistic: compensator requires normalized event time",
		)

		return input
	}

	lastSec, hasLastSec := input.Get(symbolCompensatorLastSec)
	lastNsec, hasLastNsec := input.Get(symbolCompensatorLastNsec)

	if !hasLastSec || !hasLastNsec {
		input.Put(symbolCompensatorLastSec, sec)
		input.Put(symbolCompensatorLastNsec, nsec)
		input.Put(SymbolCompensatorAlpha, 0)
		input.Put(SymbolCompensatorBeta, 0)
		input.Put(SymbolReady, 0)

		return input
	}

	delta := elapsedSince(sec, nsec, lastSec, lastNsec)

	if delta < 0 {
		input.Err = fmt.Errorf(
			"statistic: compensator event time must not regress",
		)

		return input
	}

	beta := value(input, SymbolBeta, 1)
	mark, _ := input.Get(SymbolMark)
	lambdaAlpha, _ := input.Get(SymbolLambdaAlpha)
	lambdaBeta, _ := input.Get(SymbolLambdaBeta)
	muAlpha, _ := input.Get(SymbolMuAlpha)
	muBeta, _ := input.Get(SymbolMuBeta)

	prevAlpha := undecay(lambdaAlpha, muAlpha, beta, delta)
	prevBeta := undecay(lambdaBeta, muBeta, beta, delta)

	if mark > 0 {
		prevAlpha -= value(input, SymbolAlphaAA, 0)
	} else {
		prevBeta -= value(input, SymbolAlphaBB, 0)
	}

	prevAlpha = math.Max(prevAlpha, muAlpha)
	prevBeta = math.Max(prevBeta, muBeta)

	compensatorAlpha, _ := input.Get(SymbolCompensatorAlpha)
	compensatorBeta, _ := input.Get(SymbolCompensatorBeta)
	compensatorAlpha += prevAlpha * delta
	compensatorBeta += prevBeta * delta

	alphaCount, _ := input.Get(SymbolAlphaEventCount)
	betaCount, _ := input.Get(SymbolBetaEventCount)
	innovationAlpha := alphaCount - compensatorAlpha
	innovationBeta := betaCount - compensatorBeta
	standardAlpha := 0.0
	standardBeta := 0.0

	if compensatorAlpha > 0 {
		standardAlpha = innovationAlpha / math.Sqrt(compensatorAlpha)
	}

	if compensatorBeta > 0 {
		standardBeta = innovationBeta / math.Sqrt(compensatorBeta)
	}

	input.Put(symbolCompensatorLastSec, sec)
	input.Put(symbolCompensatorLastNsec, nsec)
	input.Put(SymbolCompensatorAlpha, compensatorAlpha)
	input.Put(SymbolCompensatorBeta, compensatorBeta)
	input.Put(SymbolInnovationAlpha, innovationAlpha)
	input.Put(SymbolInnovationBeta, innovationBeta)
	input.Put(SymbolStandardInnovAlpha, standardAlpha)
	input.Put(SymbolStandardInnovBeta, standardBeta)
	input.Put(SymbolReady, 1)

	return input
}

/*
undecay inverts one event-time decay step: it recovers the intensity that held
before the last interval by re-exponentiating the current excess. The caller
subtracts the just-arrived jump to obtain the intensity that prevailed during
the interval itself.
*/
func undecay(lambda float64, baseline float64, beta float64, delta float64) float64 {
	excess := math.Max(lambda-baseline, 0)

	return baseline + excess*math.Exp(beta*delta)
}
