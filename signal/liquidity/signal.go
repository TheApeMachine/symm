package liquidity

import (
	"context"
	"math"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type welfordBaseline struct {
	count float64
	mean  float64
	m2    float64
}

/*
Step reports value against the baseline's state before it, then incorporates
it. The center is defined from the second value; the scale only once
core.PriorScale admits the prior dispersion (enough prior samples, a scale not
negligible next to the values).
*/
func (wb *welfordBaseline) Step(value float64) (
	hasCenter bool, center float64, hasScale bool, scale float64,
) {
	priorCount := wb.count
	priorMean := wb.mean
	priorM2 := wb.m2

	wb.count++
	delta := value - wb.mean
	wb.mean += delta / wb.count
	wb.m2 += delta * (value - wb.mean)

	if priorCount == 0 {
		return false, 0, false, 0
	}

	scale, hasScale = core.PriorScale(priorCount, priorM2, value, priorMean)

	return true, priorMean, hasScale, scale
}

/*
velocityTracker differentiates successive defined values over venue time; the
velocity is undefined for the first value and for an interval the venue clock
does not resolve (core.Resolvable).
*/
type velocityTracker struct {
	hasPrev   bool
	prevVal   float64
	prevAtSec float64
}

func (vt *velocityTracker) Step(value float64, atSec float64) (float64, bool) {
	hadPrev, prevVal, prevAtSec := vt.hasPrev, vt.prevVal, vt.prevAtSec
	vt.hasPrev = true
	vt.prevVal = value
	vt.prevAtSec = atSec

	if !hadPrev || !core.Resolvable(atSec-prevAtSec) {
		return 0, false
	}

	return (value - prevVal) / (atSec - prevAtSec), true
}

type symbolState struct {
	lastMeasurement *data.Measurement
	bidBaseline      welfordBaseline
	askBaseline      welfordBaseline
	spreadBaseline   welfordBaseline
	bidVel           velocityTracker
	askVel           velocityTracker
	spreadVel        velocityTracker
	historyPoints    [][2]float64
	historyDistances []float64
}

type Signal struct {
	*runtime.System
	mu     sync.Mutex
	states map[string]*symbolState
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	// The touch is the one ingress stamped onto the trade at its sequence
	// number, so this consumer sees the book the trade happened against. A
	// trade stamped without a book has no touch to measure.
	snapshot, err := broker.ReadSnapshot(prior)

	if err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] stamped book unreadable", err))
		return nil
	}

	if snapshot == nil {
		signal.mu.Lock()
		state, exists := signal.states[prior.Label]
		signal.mu.Unlock()

		if exists && state.lastMeasurement != nil {
			return state.lastMeasurement
		}

		return nil
	}

	bestBid, bestAsk := snapshot.Touch()
	bid, bidQty := bestBid.Price.Float64(), bestBid.Quantity.Float64()
	ask, askQty := bestAsk.Price.Float64(), bestAsk.Quantity.Float64()

	atSec := float64(prior.At.UnixNano()) * 1e-9

	signal.mu.Lock()
	defer signal.mu.Unlock()

	state, exists := signal.states[prior.Label]
	if !exists {
		state = &symbolState{}
		signal.states[prior.Label] = state
	}

	bidNotional := bid * bidQty
	askNotional := ask * askQty
	midpoint := (bid + ask) / 2.0
	spread := ask - bid
	relativeSpread := spread / midpoint
	twoSidedNotional := math.Min(bidNotional, askNotional)
	total := bidNotional + askNotional
	imbalance := (bidNotional - askNotional) / total

	out := map[string]float64{
		"best_bid_price":           bid,
		"best_ask_price":           ask,
		"touch_quantity:bid":       bidQty,
		"touch_quantity:ask":       askQty,
		"touch_notional:bid":       bidNotional,
		"touch_notional:ask":       askNotional,
		"midpoint":                 midpoint,
		"spread":                   spread,
		"relative_spread":          relativeSpread,
		"two_sided_touch_notional": twoSidedNotional,
		"touch_notional_imbalance": imbalance,
	}

	// Each channel's baseline, ratio, divergence, noise scale, z-score, and
	// divergence velocity are written only where defined: the baseline from
	// the second value, the ratio while the baseline is not negligible next
	// to the value, the z-score once the prior scale is admitted, and the
	// velocity over a resolvable interval. Undefined is absent, never zero.
	channel := func(
		baseline *welfordBaseline, velocity *velocityTracker, value float64,
		center, ratio, divergence, scale, zscore, rate string,
	) (float64, bool) {
		hasCenter, priorCenter, hasScale, priorScale := baseline.Step(value)

		if !hasCenter {
			return 0, false
		}

		div := value - priorCenter
		out[center] = priorCenter
		out[divergence] = div

		if priorCenter > 0 && !core.Negligible(priorCenter, value) {
			out[ratio] = value / priorCenter
		}

		if vel, ok := velocity.Step(div, atSec); ok {
			out[rate] = vel
		}

		if !hasScale {
			return 0, false
		}

		z := div / priorScale
		out[scale] = priorScale
		out[zscore] = z

		return z, true
	}

	channel(
		&state.bidBaseline, &state.bidVel, bidNotional, "touch_notional_baseline:bid",
		"depth_ratio:bid", "depth_divergence:bid", "depth_noise_scale:bid",
		"depth_zscore:bid", "divergence_velocity:bid",
	)
	channel(
		&state.askBaseline, &state.askVel, askNotional, "touch_notional_baseline:ask",
		"depth_ratio:ask", "depth_divergence:ask", "depth_noise_scale:ask",
		"depth_zscore:ask", "divergence_velocity:ask",
	)
	spreadZ, hasSpreadZ := channel(
		&state.spreadBaseline, &state.spreadVel, relativeSpread, "relative_spread_baseline",
		"spread_ratio", "spread_divergence", "spread_noise_scale",
		"spread_zscore", "spread_divergence_velocity",
	)

	// The historical path lives in (spread z-score, imbalance) space, so a
	// frame without a spread z-score has no point on it.
	if hasSpreadZ {
		state.historyPath(out, [2]float64{spreadZ, imbalance})
	}

	if state.lastMeasurement == nil {
		state.lastMeasurement = data.NewFromPrior(prior, signal.Name(), out)
		return state.lastMeasurement
	}

	state.lastMeasurement = state.lastMeasurement.Next(out)
	state.lastMeasurement.At = prior.At
	return state.lastMeasurement
}

/*
historyPath writes the distance from target to the nearest retained path point
and that distance's percentile among the retained distances, each once it is
defined, then retains target.
*/
func (state *symbolState) historyPath(out map[string]float64, target [2]float64) {
	if len(state.historyPoints) > 0 {
		minDist := math.Inf(1)

		for _, point := range state.historyPoints {
			dx := target[0] - point[0]
			dy := target[1] - point[1]
			minDist = math.Min(minDist, math.Sqrt(dx*dx+dy*dy))
		}

		if len(state.historyDistances) > 0 {
			belowCount := 0

			for _, pastDist := range state.historyDistances {
				if pastDist <= minDist {
					belowCount++
				}
			}

			out["historical_path_percentile"] = float64(belowCount) / float64(len(state.historyDistances))
		}

		out["historical_path_distance"] = minDist
		state.historyDistances = append(state.historyDistances, minDist)

		if len(state.historyDistances) > 256 {
			state.historyDistances = state.historyDistances[len(state.historyDistances)-256:]
		}
	}

	state.historyPoints = append(state.historyPoints, target)

	if len(state.historyPoints) > 256 {
		state.historyPoints = state.historyPoints[len(state.historyPoints)-256:]
	}
}
