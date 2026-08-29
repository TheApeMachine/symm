package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolSlope                 = types.MustIntern("slope/beta")
	SymbolSlopeVariance         = types.MustIntern("slope/variance")
	SymbolSlopeSNR              = types.MustIntern("slope/snr")
	SymbolSlopeIntercept        = types.MustIntern("slope/intercept")
	SymbolSlopeResidualVariance = types.MustIntern("slope/residual_variance")
	// SymbolSlopeHorizon is the optional causally derived horizon (seconds).
	// When present, only prior path observations whose event time is within
	// window = [now - horizon, now) participate in the fit. When absent, the
	// whole retained path participates (backward-compatible).
	SymbolSlopeHorizon = types.MustIntern("slope/horizon")
)

type slopeSlots struct {
	slope            types.Symbol
	variance         types.Symbol
	snr              types.Symbol
	intercept        types.Symbol
	residualVariance types.Symbol
	lastSec          types.Symbol
	lastNsec         types.Symbol
}

func newSlopeSlots(prefix string) slopeSlots {
	return slopeSlots{
		slope:            types.MustIntern(temporal.JoinPrefix(prefix, "slope/beta")),
		variance:         types.MustIntern(temporal.JoinPrefix(prefix, "slope/variance")),
		snr:              types.MustIntern(temporal.JoinPrefix(prefix, "slope/snr")),
		intercept:        types.MustIntern(temporal.JoinPrefix(prefix, "slope/intercept")),
		residualVariance: types.MustIntern(temporal.JoinPrefix(prefix, "slope/residual_variance")),
		lastSec:          types.MustIntern(temporal.JoinPrefix(prefix, "slope/last_sec")),
		lastNsec:         types.MustIntern(temporal.JoinPrefix(prefix, "slope/last_nsec")),
	}
}

/*
Slope computes the causal ordinary-least-squares local linear regression over
one series' retained temporal path observations:

	x_i = a + beta * (t_i - t) + epsilon_i

where t is the current observation event time, a is the level intercept at
time t, beta is the instantaneous rate of change (value per second), and
epsilon_i are the causal empirical residuals.

The prefix namespaces every slot; the empty prefix uses the generic slope slots.
*/
func Slope(prefix string) types.Primitive {
	series := temporal.NewSeries(prefix)
	slots := newSlopeSlots(prefix)

	return func(input *types.Frame) {
		_, hasValue := input.Get(series.ValueSymbol)
		sec, hasSec := input.Get(series.SecSymbol)
		nsec, hasNsec := input.Get(series.NsecSymbol)

		if !hasValue || !hasSec || !hasNsec {
			input.Err = fmt.Errorf("statistic: slope requires a value and event time")

			return
		}

		if nsec < 0 || nsec >= 1e9 {
			input.Err = fmt.Errorf("statistic: slope requires normalized nanoseconds")

			return
		}

		previousSec, hasLastSec := input.Get(slots.lastSec)
		previousNsec, hasLastNsec := input.Get(slots.lastNsec)

		if hasLastSec && hasLastNsec {
			if elapsedSince(sec, nsec, previousSec, previousNsec) < 0 {
				input.Err = fmt.Errorf("statistic: slope event time must not regress")

				return
			}
		}

		input.Put(slots.lastSec, sec)
		input.Put(slots.lastNsec, nsec)

		count := series.Count(*input)

		if count < 2 {
			// Undefined ≠ zero: while the regression has no support, the slope
			// (and auxiliary estimate slots) are deleted so consuming projectors
			// observe absence rather than a fabricated numeric 0.
			input.Delete(slots.slope)
			input.Delete(slots.intercept)
			input.Delete(slots.variance)
			input.Delete(slots.snr)
			input.Delete(slots.residualVariance)
			input.Put(series.ReadySymbol, 0)

			return
		}

		currentTimestamp := int64(sec)*1_000_000_000 + int64(nsec)

		// Optional causally derived horizon: restrict the fit to prior
		// observations within [now - horizon, now).
		horizonSeconds, hasHorizon := input.Get(SymbolSlopeHorizon)
		cutoff := int64(0)

		if hasHorizon && horizonSeconds > 0 {
			cutoff = currentTimestamp - int64(horizonSeconds*1e9)
		}

		slope, intercept, slopeVar, snr, resVar, ok := fitLocalRegression(series, input, count, currentTimestamp, cutoff)

		if !ok {
			// Undefined ≠ zero: the fit left no defined slope, so delete the
			// estimate slots rather than emit a numeric zero that a downstream
			// projector would misread as a real, defined velocity.
			input.Delete(slots.slope)
			input.Delete(slots.intercept)
			input.Delete(slots.variance)
			input.Delete(slots.snr)
			input.Delete(slots.residualVariance)
			input.Put(series.ReadySymbol, 0)

			return
		}

		input.Put(slots.slope, slope)
		input.Put(slots.intercept, intercept)
		input.Put(slots.variance, slopeVar)
		input.Put(slots.snr, snr)
		input.Put(slots.residualVariance, resVar)
		input.Put(series.ReadySymbol, 1)
	}
}

/*
LocalRegression is an alias for Slope to match terminology in domain specifications.
*/
func LocalRegression(prefix string) types.Primitive {
	return Slope(prefix)
}

func fitLocalRegression(
	series temporal.Series,
	frame *types.Frame,
	count int,
	currentTimestamp int64,
	cutoff int64,
) (float64, float64, float64, float64, float64, bool) {
	// First collect the in-window samples (and their count), then fit. The
	// current observation is never in the retained path (temporal.Path appends
	// after Slope), so no explicit "exclude self" step is needed; the horizon
	// only further restricts which PRIOR samples participate.
	type windowed struct {
		timestamp int64
		value     float64
	}

	samples := make([]windowed, 0, count)

	for index := range count {
		timestamp, sampleValue, found := series.Sample(frame, index)

		if !found {
			return 0, 0, 0, 0, 0, false
		}

		if cutoff > 0 && timestamp < cutoff {
			continue
		}

		samples = append(samples, windowed{timestamp: timestamp, value: sampleValue})
	}

	sampleCount := float64(len(samples))

	if sampleCount < 2 {
		return 0, 0, 0, 0, 0, false
	}

	sumTau := 0.0
	sumX := 0.0

	for _, sample := range samples {
		tau := float64(sample.timestamp-currentTimestamp) / 1e9
		sumTau += tau
		sumX += sample.value
	}

	meanTau := sumTau / sampleCount
	meanX := sumX / sampleCount

	sumTauTau := 0.0
	sumTauX := 0.0
	sumXX := 0.0

	for _, sample := range samples {
		tau := float64(sample.timestamp-currentTimestamp) / 1e9
		deltaTau := tau - meanTau
		deltaX := sample.value - meanX

		sumTauTau += deltaTau * deltaTau
		sumTauX += deltaTau * deltaX
		sumXX += deltaX * deltaX
	}

	if sumTauTau <= 0 || math.IsNaN(sumTauTau) {
		return 0, 0, 0, 0, 0, false
	}

	slope := sumTauX / sumTauTau
	intercept := meanX - slope*meanTau

	sumSquaredErrors := sumXX - slope*sumTauX

	if sumSquaredErrors < 0 {
		sumSquaredErrors = 0
	}

	degreesOfFreedom := sampleCount - 2
	residualVariance := 0.0
	slopeVariance := 0.0
	slopeSNR := 0.0

	if degreesOfFreedom > 0 {
		residualVariance = sumSquaredErrors / degreesOfFreedom
		slopeVariance = residualVariance / sumTauTau

		if slopeVariance > 0 {
			slopeSNR = (slope * slope) / slopeVariance
		}
	}

	return slope, intercept, slopeVariance, slopeSNR, residualVariance, true
}
