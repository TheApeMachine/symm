package toxicity

import (
	"context"
	"fmt"
	"math"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
welfordBaseline is a causal baseline: each value is scored against the state
before it. The center is defined from the second value, the scale once the
prior dispersion is positive.
*/
type welfordBaseline struct {
	count float64
	mean  float64
	m2    float64
}

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

	scale, scorable := core.PriorScale(priorCount, priorM2, value, priorMean)

	return true, priorMean, scorable, scale
}

/*
emit writes name's baseline, divergence, and z-score for value as far as the
baseline defines them, and returns the z-score with its definedness.
*/
func (wb *welfordBaseline) emit(
	out map[string]float64, baseline, divergence, zscore string, value float64,
) (float64, bool) {
	hasCenter, center, hasScale, scale := wb.Step(value)

	if !hasCenter {
		return 0, false
	}

	out[baseline] = center

	if divergence != "" {
		out[divergence] = value - center
	}

	if !hasScale || zscore == "" {
		return 0, false
	}

	z := (value - center) / scale
	out[zscore] = z

	return z, true
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

/*
sideBracket is one touch side's attribution over the bracket (t0, t1]: the
previous touch (P0, Q0), the current touch (P1, Q1), and the raw quantity E*
that aggressive trades executed at P0.
*/
type sideBracket struct {
	prevPrice, prevQty float64
	price, qty         float64
	matched            float64
	improves           func(price, prevPrice float64) bool
}

/*
dispose writes the side's attribution, per the specification: E = min(E*, Q0),
U = Q0 - E; at an unchanged price W = max(U - Q1, 0) and A = max(Q1 - U, 0)
with zero retreat; on a retreat R = U with no same-price disposition; on an
improvement the previous level's disposition is unresolved and left undefined.
Rates divide by the bracket's duration dt and are undefined when the venue
clock does not resolve it (core.Resolvable).
*/
func (side sideBracket) dispose(out map[string]float64, suffix string, dt float64) {
	rated := core.Resolvable(dt)
	fill := math.Min(side.matched, side.prevQty)
	unfilled := side.prevQty - fill

	out["previous_best_price"+suffix] = side.prevPrice
	out["previous_touch_quantity"+suffix] = side.prevQty
	out["touch_price_log_change"+suffix] = math.Log(side.price / side.prevPrice)
	out["matched_touch_trade_quantity"+suffix] = side.matched
	out["touch_fill_quantity"+suffix] = fill
	out["touch_fill_fraction"+suffix] = fill / side.prevQty

	if rated {
		out["touch_fill_rate"+suffix] = fill / dt
	}

	out["unfilled_residual_quantity"+suffix] = unfilled

	switch {
	case side.price == side.prevPrice:
		withdrawn := math.Max(unfilled-side.qty, 0)
		replenished := math.Max(side.qty-unfilled, 0)

		out["net_withdrawn_quantity"+suffix] = withdrawn
		out["net_withdrawal_fraction"+suffix] = withdrawn / side.prevQty
		out["net_replenished_quantity"+suffix] = replenished
		out["net_replenishment_fraction"+suffix] = replenished / side.prevQty
		out["retreated_quantity"+suffix] = 0
		out["retreat_fraction"+suffix] = 0

		if rated {
			out["net_withdrawal_rate"+suffix] = withdrawn / dt
			out["net_replenishment_rate"+suffix] = replenished / dt
			out["retreat_rate"+suffix] = 0
		}
	case !side.improves(side.price, side.prevPrice):
		out["retreated_quantity"+suffix] = unfilled
		out["retreat_fraction"+suffix] = unfilled / side.prevQty

		if rated {
			out["retreat_rate"+suffix] = unfilled / dt
		}
	}
}

type symbolState struct {
	hasPrev                      bool
	prevBid                      float64
	prevAsk                      float64
	prevBidQty                   float64
	prevAskQty                   float64
	prevAtNano                   float64
	prevAt                       time.Time
	pendingTradeQty              float64
	pendingMatchedBid            float64
	pendingMatchedAsk            float64
	fillFracBidBaseline          welfordBaseline
	fillFracAskBaseline          welfordBaseline
	withdrawalFracBidBaseline    welfordBaseline
	withdrawalFracAskBaseline    welfordBaseline
	retreatFracBidBaseline       welfordBaseline
	retreatFracAskBaseline       welfordBaseline
	replenishmentFracBidBaseline welfordBaseline
	replenishmentFracAskBaseline welfordBaseline
	fillFracBidVel               velocityTracker
	fillFracAskVel               velocityTracker
	withdrawalFracBidVel         velocityTracker
	withdrawalFracAskVel         velocityTracker
	historyPoints                [][6]float64
	historyDistances             []float64
}

type Signal struct {
	*runtime.System
	books  broker.BookSource
	states map[string]*symbolState
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books:  books,
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "toxicity", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[toxicity] book manager is required", err))
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
		signal.Error(errnie.Err(errnie.Internal, "[toxicity] book manager is required", nil))
		return nil
	}

	price, err := tradeValue(prior, "price")
	if err != nil {
		signal.Error(err)
		return nil
	}

	qty, err := tradeValue(prior, "qty")
	if err != nil {
		signal.Error(err)
		return nil
	}

	if price <= 0 || qty <= 0 {
		return nil
	}

	var bid, ask, bidQty, askQty float64
	var found bool

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil || book.BestBid() == nil || book.BestAsk() == nil {
			return
		}

		bid, bidQty = book.BestBid().Price.Float64(), book.BestBid().Quantity.Float64()
		ask, askQty = book.BestAsk().Price.Float64(), book.BestAsk().Quantity.Float64()
		found = true
	})

	if !found {
		return nil
	}

	if bid <= 0 || ask <= 0 || bidQty <= 0 || askQty <= 0 || math.IsNaN(bid) || math.IsNaN(ask) || math.IsNaN(bidQty) || math.IsNaN(askQty) || math.IsInf(bid, 0) || math.IsInf(ask, 0) || math.IsInf(bidQty, 0) || math.IsInf(askQty, 0) {
		signal.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("[%s] %s: non-finite or non-positive book touch: bid=%v ask=%v bidQty=%v askQty=%v", signal.Name(), prior.Label, bid, ask, bidQty, askQty),
			nil,
		))
		return nil
	}

	if ask <= bid {
		errnie.Warn(fmt.Sprintf(
			"[toxicity] dropping frame for %s with crossed book: bid=%v ask=%v",
			prior.Label, bid, ask,
		))

		return nil
	}

	sideIndicator := 0.0
	side := prior.Meta("side")

	if side == "buy" {
		sideIndicator = 1.0
	}

	if side == "sell" {
		sideIndicator = -1.0
	}

	atNano := float64(prior.At.UnixNano())
	atSec := atNano / 1e9

	state, exists := signal.states[prior.Label]

	if !exists {
		state = &symbolState{}
		signal.states[prior.Label] = state
	}

	out := map[string]float64{
		"best_price:bid":     bid,
		"best_price:ask":     ask,
		"touch_quantity:bid": bidQty,
		"touch_quantity:ask": askQty,
	}

	// The first observation opens the first bracket: its trade executed at or
	// before t0 and is not attributed.
	if !state.hasPrev {
		state.observe(bid, ask, bidQty, askQty, atNano, prior.At)
		return prior.Next(signal.Name(), out)
	}

	// Aggressive buys execute the previous ask, aggressive sells the
	// previous bid. Trades sharing the previous observation's timestamp cannot
	// close a bracket, so they accrue to the next one.
	state.pendingTradeQty += qty

	if sideIndicator > 0 && price == state.prevAsk {
		state.pendingMatchedAsk += qty
	}

	if sideIndicator < 0 && price == state.prevBid {
		state.pendingMatchedBid += qty
	}

	if atNano <= state.prevAtNano {
		return prior.Next(signal.Name(), out)
	}

	dt := (atNano - state.prevAtNano) / 1e9
	out["bracket_trade_quantity"] = state.pendingTradeQty

	sideBracket{
		prevPrice: state.prevBid, prevQty: state.prevBidQty,
		price: bid, qty: bidQty, matched: state.pendingMatchedBid,
		improves: func(price, prevPrice float64) bool { return price > prevPrice },
	}.dispose(out, ":bid", dt)
	sideBracket{
		prevPrice: state.prevAsk, prevQty: state.prevAskQty,
		price: ask, qty: askQty, matched: state.pendingMatchedAsk,
		improves: func(price, prevPrice float64) bool { return price < prevPrice },
	}.dispose(out, ":ask", dt)

	from := state.prevAt
	state.observe(bid, ask, bidQty, askQty, atNano, prior.At)

	fillBidZ, hasFillBidZ := state.fillFracBidBaseline.emit(
		out, "fill_fraction_baseline:bid", "fill_fraction_divergence:bid",
		"fill_fraction_zscore:bid", out["touch_fill_fraction:bid"],
	)
	fillAskZ, hasFillAskZ := state.fillFracAskBaseline.emit(
		out, "fill_fraction_baseline:ask", "fill_fraction_divergence:ask",
		"fill_fraction_zscore:ask", out["touch_fill_fraction:ask"],
	)

	if velocity, ok := state.fillFracBidVel.Step(out["touch_fill_fraction:bid"], atSec); ok {
		out["fill_fraction_velocity:bid"] = velocity
	}

	if velocity, ok := state.fillFracAskVel.Step(out["touch_fill_fraction:ask"], atSec); ok {
		out["fill_fraction_velocity:ask"] = velocity
	}

	var withdrawBidZ, withdrawAskZ, retreatBidZ, retreatAskZ float64
	var hasWithdrawBidZ, hasWithdrawAskZ, hasRetreatBidZ, hasRetreatAskZ bool

	if fraction, ok := out["net_withdrawal_fraction:bid"]; ok {
		withdrawBidZ, hasWithdrawBidZ = state.withdrawalFracBidBaseline.emit(
			out, "withdrawal_fraction_baseline:bid", "withdrawal_fraction_divergence:bid",
			"withdrawal_fraction_zscore:bid", fraction,
		)

		if velocity, defined := state.withdrawalFracBidVel.Step(fraction, atSec); defined {
			out["withdrawal_fraction_velocity:bid"] = velocity
		}

		state.replenishmentFracBidBaseline.emit(
			out, "replenishment_fraction_baseline:bid", "", "", out["net_replenishment_fraction:bid"],
		)
	}

	if fraction, ok := out["net_withdrawal_fraction:ask"]; ok {
		withdrawAskZ, hasWithdrawAskZ = state.withdrawalFracAskBaseline.emit(
			out, "withdrawal_fraction_baseline:ask", "withdrawal_fraction_divergence:ask",
			"withdrawal_fraction_zscore:ask", fraction,
		)

		if velocity, defined := state.withdrawalFracAskVel.Step(fraction, atSec); defined {
			out["withdrawal_fraction_velocity:ask"] = velocity
		}

		state.replenishmentFracAskBaseline.emit(
			out, "replenishment_fraction_baseline:ask", "", "", out["net_replenishment_fraction:ask"],
		)
	}

	if fraction, ok := out["retreat_fraction:bid"]; ok {
		retreatBidZ, hasRetreatBidZ = state.retreatFracBidBaseline.emit(
			out, "retreat_fraction_baseline:bid", "", "retreat_fraction_zscore:bid", fraction,
		)
	}

	if fraction, ok := out["retreat_fraction:ask"]; ok {
		retreatAskZ, hasRetreatAskZ = state.retreatFracAskBaseline.emit(
			out, "retreat_fraction_baseline:ask", "", "retreat_fraction_zscore:ask", fraction,
		)
	}

	if hasFillBidZ && hasFillAskZ && hasWithdrawBidZ && hasWithdrawAskZ &&
		hasRetreatBidZ && hasRetreatAskZ {
		state.historyPath(out, [6]float64{
			fillBidZ, fillAskZ, withdrawBidZ, withdrawAskZ, retreatBidZ, retreatAskZ,
		})
	}

	res := prior.Next(signal.Name(), out)

	// The attributed bracket is (t0, t1].
	res.From = from

	return res
}

/*
observe closes the bracket at the current touch: it becomes the previous
touch the next bracket attributes against.
*/
func (state *symbolState) observe(
	bid, ask, bidQty, askQty, atNano float64, at time.Time,
) {
	state.hasPrev = true
	state.prevBid = bid
	state.prevAsk = ask
	state.prevBidQty = bidQty
	state.prevAskQty = askQty
	state.prevAtNano = atNano
	state.prevAt = at
	state.pendingTradeQty = 0
	state.pendingMatchedBid = 0
	state.pendingMatchedAsk = 0
}

/*
historyPath scores how far this bracket's six disposition z-scores lie from
the nearest retained point, and that distance's rank among past nearest
distances.
*/
func (state *symbolState) historyPath(out map[string]float64, target [6]float64) {
	if len(state.historyPoints) > 0 {
		minDist := math.Inf(1)

		for _, point := range state.historyPoints {
			var sumSq float64

			for dim := range target {
				diff := target[dim] - point[dim]
				sumSq += diff * diff
			}

			minDist = math.Min(minDist, math.Sqrt(sumSq))
		}

		out["historical_path_distance"] = minDist

		if len(state.historyDistances) > 0 {
			below := 0

			for _, past := range state.historyDistances {
				if past <= minDist {
					below++
				}
			}

			out["historical_path_percentile"] = float64(below) / float64(len(state.historyDistances))
		}

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

func tradeValue(prior *data.Measurement, key string) (float64, error) {
	entry := data.Pull(prior.Read(key))

	if entry != nil && entry.Err != nil {
		return 0, entry.Err
	}

	if entry == nil || entry.Metric == nil || entry.Metric.Label != key {
		return 0, errnie.Err(
			errnie.NotAcceptable, "[toxicity] trade frame is missing "+key, nil,
		)
	}

	return entry.Metric.Raw, nil
}
