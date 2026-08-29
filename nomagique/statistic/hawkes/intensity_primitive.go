package hawkes

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Observation-scoped Frame facts recomputed on every call. Unlike Model*, these
never persist a value across calls when the model is not yet fitted: Reset
clears them at the start of every transition so a not-ready event cannot
republish a stale prior event's numbers, per signal/hawkes/README.md section
32 ("failed fits do not overwrite the last committed valid state" — applied
here to observation output, which is not committed state at all).
*/
var (
	SymbolConditionalIntensityBuy  = types.MustIntern("hawkes/obs/conditional_intensity_buy")
	SymbolConditionalIntensitySell = types.MustIntern("hawkes/obs/conditional_intensity_sell")
	SymbolBackgroundRateBuy        = types.MustIntern("hawkes/obs/background_rate_buy")
	SymbolBackgroundRateSell       = types.MustIntern("hawkes/obs/background_rate_sell")
)

/*
ConditionalIntensity evaluates the pre-arrival intensities λ_b(t⁻), λ_s(t⁻)
from the model fitted before this event (Fit has not run yet in the composed
Pipe — see algo.Hawkes) and the arrival path retained before this event
(ArrivalPath has not run yet either). It requires a converged fit: per
README section 22, "lack of a usable fit is represented as undefined fitted
metrics, not zero parameters," so this primitive intentionally leaves its
output symbols absent rather than substituting mu=0/alpha=0 when no fit
exists yet.
*/
func ConditionalIntensity(input *types.Frame) {
	muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta, ok := ReadModel(input)

	if !ok {
		return
	}

	buy, sell, ok := retainedArrivals(input)

	if !ok {
		buy, sell = nil, nil
	}

	horizonSec := eventHorizonSec(input)
	lambdaBuy := intensityAt(buy, sell, horizonSec, muX, alphaXX, alphaXY, beta)
	lambdaSell := intensityAt(buy, sell, horizonSec, muY, alphaYX, alphaYY, beta)

	if !finite(lambdaBuy, lambdaSell) {
		input.Err = fmt.Errorf("hawkes: conditional intensity must be finite")

		return
	}

	input.Put(SymbolConditionalIntensityBuy, lambdaBuy)
	input.Put(SymbolConditionalIntensitySell, lambdaSell)
	input.Put(SymbolBackgroundRateBuy, muX)
	input.Put(SymbolBackgroundRateSell, muY)
}
