package leadlag

import (
	"math"
	"time"

	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	anchorMoveMinObs = 12
	anchorMoveAlpha  = 0.05
	// anchorMoveMinLogReturn regularizes the adaptive move floor for hourly log-return
	// magnitudes, analogous to SNR minStd on the unit interval.
	anchorMoveMinLogReturn = 1e-5
)

/*
anchorMove is the anchor path verdict over the lag search window.
*/
type anchorMove struct {
	moved       bool
	stallMargin float64
	ready       bool
}

/*
moveBaseline tracks exponentially weighted anchor path moves and scores the
latest reading against that history with a regularized noise floor.
*/
type moveBaseline struct {
	moments adaptive.EWMoments
	minObs  int
	alpha   float64
	minMove float64
}

func newMoveBaseline() *moveBaseline {
	return &moveBaseline{
		minObs:  anchorMoveMinObs,
		alpha:   anchorMoveAlpha,
		minMove: anchorMoveMinLogReturn,
	}
}

func anchorMoveWindow() time.Duration {
	return time.Duration(maxLagBars) * barInterval
}

/*
recentPathMove returns the absolute log return from the oldest sample inside
window to the latest tick. It fails when the ring cannot cover half the window.
*/
func (state *symbolState) recentPathMove(window time.Duration) (float64, bool) {
	samples := state.priceSamples()

	if len(samples) < minLagSamples || window <= 0 {
		return 0, false
	}

	latest := samples[len(samples)-1]
	cutoff := latest.At.Add(-window)

	startIndex := -1

	for index, sample := range samples {
		if !sample.At.Before(cutoff) {
			startIndex = index

			break
		}
	}

	if startIndex < 0 {
		return 0, false
	}

	start := samples[startIndex]

	if start.Price <= 0 || latest.Price <= 0 {
		return 0, false
	}

	if latest.At.Sub(start.At) < window/2 {
		return 0, false
	}

	return math.Abs(math.Log(latest.Price / start.Price)), true
}

/*
evaluate scores recentMove against the running baseline. ready is false while
the baseline is still warming up.
*/
func (baseline *moveBaseline) evaluate(recentMove float64) (moved bool, stallMargin float64, ready bool) {
	if baseline.moments.Observations() < baseline.minObs {
		_ = baseline.moments.Update(recentMove, baseline.alpha)

		return false, 0, false
	}

	mean := baseline.moments.Mean()
	historicalVar := baseline.moments.VarianceEWMA()

	if historicalVar < 0 {
		historicalVar = 0
	}

	floorVar := baseline.minMove * baseline.minMove
	threshold := mean + math.Sqrt(historicalVar+floorVar)

	moved = recentMove > threshold

	if !moved && threshold > 0 {
		stallMargin = (threshold - recentMove) / threshold
	}

	_ = baseline.moments.Update(recentMove, baseline.alpha)

	return moved, stallMargin, true
}

func (signal *Signal) anchorMoveStatus(anchor *symbolState) anchorMove {
	recentMove, ok := anchor.recentPathMove(anchorMoveWindow())

	if !ok {
		return anchorMove{}
	}

	moved, stallMargin, ready := signal.anchorBaseline.evaluate(recentMove)

	return anchorMove{
		moved:       moved,
		stallMargin: stallMargin,
		ready:       ready,
	}
}
