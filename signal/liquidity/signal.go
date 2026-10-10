package liquidity

import (
	"context"
	"fmt"
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
	books  broker.BookHistory
	mu     sync.Mutex
	states map[string]*symbolState
}

func NewSignal(ctx context.Context, books broker.BookHistory) *Signal {
	signal := &Signal{
		books:  books,
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] book manager is required", err))
	}

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

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] book manager is required", nil))
		return nil
	}

	var bid, ask, bidQty, askQty float64
	var found bool

	// The book as of this frame, not as the live book stands when the frame
	// is processed.
	signal.books.BookAt(prior.Label, prior.At, func(book *broker.BookView) {
		if !book.Complete || len(book.Bids) == 0 || len(book.Asks) == 0 {
			return
		}

		bid, bidQty = book.Bids[0].Price, book.Bids[0].Quantity
		ask, askQty = book.Asks[0].Price, book.Asks[0].Quantity
		found = true
	})

	if !found {
		return nil
	}

	touch := map[string]float64{
		"bid":     bid,
		"ask":     ask,
		"bid_qty": bidQty,
		"ask_qty": askQty,
	}

	for _, value := range touch {
		if !broker.ValidTouchValue(value) {
			signal.Error(broker.InvalidTouch("liquidity", prior.Label, touch))
			return nil
		}
	}

	if ask <= bid {
		errnie.Warn(fmt.Sprintf("[liquidity] dropping frame for %s with crossed book: bid=%v ask=%v", prior.Label, bid, ask))
		return nil
	}

	if bidQty <= 0 || askQty <= 0 {
		return nil
	}

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

	return prior.Next(signal.Name(), out)
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
