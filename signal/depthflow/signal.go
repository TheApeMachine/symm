package depthflow

import (
	"context"
	"fmt"
	"math"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type causalEstimator struct {
	count float64
	mean  float64
	m2    float64
}

/*
Step scores value against the estimator's state before it, then incorporates
it. The baseline (and residual against it) is defined from the second sample;
the z-score only once the prior dispersion is positive. Undefined is reported,
never substituted.
*/
func (ce *causalEstimator) Step(value float64) (
	hasBaseline bool, baseline, residual float64, hasZ bool, zScore float64,
) {
	priorCount := ce.count
	priorMean := ce.mean
	priorM2 := ce.m2

	ce.count++
	delta := value - ce.mean
	ce.mean += delta / ce.count
	ce.m2 += delta * (value - ce.mean)

	if priorCount == 0 {
		return false, 0, 0, false, 0
	}

	residual = value - priorMean

	if priorCount < 2 || priorM2 <= 0 {
		return true, priorMean, residual, false, 0
	}

	return true, priorMean, residual, true, residual / math.Sqrt(priorM2/(priorCount-1))
}

type symbolState struct {
	prevBids            map[float64]float64
	prevAsks            map[float64]float64
	prevFullBid         bool
	prevFullAsk         bool
	prevBidWorst        float64
	prevAskWorst        float64
	prevTotal           float64
	prevAtNano          float64
	prevAt              time.Time
	hasPrev             bool
	hasPrevBookImb      bool
	prevBookImb         float64
	hasPrevGap          bool
	prevGap             float64
	bookImbEstimator    causalEstimator
	gapEstimator        causalEstimator
	turnoverEstimator   causalEstimator
	netChangeEstimator  causalEstimator
	signedFlowEstimator causalEstimator
	historyPoints       [][2]float64
	historyDistances    []float64
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

	signal.System = runtime.NewSystem(ctx, "depthflow", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[depthflow] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[depthflow] book manager is required", nil))
		return nil
	}

	var bids, asks [][2]float64
	var crossedBid, crossedAsk float64
	fullBid, fullAsk := false, false
	ok := false
	crossed := false

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		bid := book.BestBid()
		ask := book.BestAsk()

		if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
			return
		}

		bidPrice := kraken.Float64(bid.Price)
		askPrice := kraken.Float64(ask.Price)

		if askPrice <= bidPrice {
			crossed = true
			crossedBid = bidPrice
			crossedAsk = askPrice
			return
		}

		// The book keeps MaxDepth levels per side. A full side hides whatever
		// lies beyond its worst level, so a price crossing that edge is a
		// window change, not displayed flow.
		fullBid = len(book.Bids.Levels) >= book.MaxDepth
		fullAsk = len(book.Asks.Levels) >= book.MaxDepth

		for cursor := bid; cursor != nil; cursor = cursor.Lower {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 {
				continue
			}

			bids = append(bids, [2]float64{price, qty})
		}

		for cursor := ask; cursor != nil; cursor = cursor.Higher {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 {
				continue
			}

			asks = append(asks, [2]float64{price, qty})
		}

		ok = len(bids) > 0 && len(asks) > 0
	})

	if crossed {
		errnie.Warn(fmt.Sprintf("[depthflow] dropping frame for %s with crossed book: bid=%v ask=%v", prior.Label, crossedBid, crossedAsk))
		return nil
	}

	if !ok {
		return nil
	}

	touchBid := bids[0][0] * bids[0][1]
	touchAsk := asks[0][0] * asks[0][1]

	obsBid := 0.0
	currentBids := make(map[float64]float64, len(bids))

	for _, b := range bids {
		notional := b[0] * b[1]
		obsBid += notional
		currentBids[b[0]] = notional
	}

	obsAsk := 0.0
	currentAsks := make(map[float64]float64, len(asks))

	for _, a := range asks {
		notional := a[0] * a[1]
		obsAsk += notional
		currentAsks[a[0]] = notional
	}

	total := obsBid + obsAsk
	bookImb := (obsBid - obsAsk) / total
	touchImb := (touchBid - touchAsk) / (touchBid + touchAsk)
	gap := touchImb - bookImb
	gapDist := math.Abs(gap)

	atNano := float64(prior.At.UnixNano())
	state, found := signal.states[prior.Label]

	if !found {
		state = &symbolState{
			prevBids: make(map[float64]float64),
			prevAsks: make(map[float64]float64),
		}
		signal.states[prior.Label] = state
	}

	out := map[string]float64{
		"book_notional:bid":             obsBid,
		"book_notional:ask":             obsAsk,
		"book_notional":                 total,
		"book_imbalance":                bookImb,
		"touch_imbalance":               touchImb,
		"imbalance_resolution_gap":      gap,
		"imbalance_resolution_distance": gapDist,
	}

	// Flow needs a prior book; rates also need elapsed time. A first frame
	// or a same-timestamp frame leaves them undefined.
	timeDelta := 0.0
	hasRate := false
	bookTurnoverRate := 0.0
	netBookChangeRate := 0.0
	signedNetFlowRate := 0.0

	if state.hasPrev {
		if atNano > state.prevAtNano {
			timeDelta = (atNano - state.prevAtNano) * 1e-9
			hasRate = true
		}

		bidWorst := bids[len(bids)-1][0]
		askWorst := asks[len(asks)-1][0]
		addedBid, removedBid := levelFlow(
			state.prevBids, currentBids, state.prevFullBid, fullBid,
			state.prevBidWorst, bidWorst, func(price, edge float64) bool { return price >= edge },
		)
		addedAsk, removedAsk := levelFlow(
			state.prevAsks, currentAsks, state.prevFullAsk, fullAsk,
			state.prevAskWorst, askWorst, func(price, edge float64) bool { return price <= edge },
		)

		netBid := addedBid - removedBid
		netAsk := addedAsk - removedAsk
		bookActivity := addedBid + removedBid + addedAsk + removedAsk
		netBookChange := total - state.prevTotal
		signedNetFlow := netBid - netAsk

		out["added_notional:bid"] = addedBid
		out["removed_notional:bid"] = removedBid
		out["net_displayed_flow:bid"] = netBid
		out["added_notional:ask"] = addedAsk
		out["removed_notional:ask"] = removedAsk
		out["net_displayed_flow:ask"] = netAsk

		if denom := math.Abs(netBid) + math.Abs(netAsk); denom > 0 {
			out["flow_activity_imbalance"] = signedNetFlow / denom
		}

		if hasRate {
			reference := (state.prevTotal + total) / 2.0
			bookTurnoverRate = bookActivity / (reference * timeDelta)
			netBookChangeRate = netBookChange / (reference * timeDelta)
			signedNetFlowRate = signedNetFlow / (reference * timeDelta)

			out["added_notional_rate:bid"] = addedBid / timeDelta
			out["added_notional_rate:ask"] = addedAsk / timeDelta
			out["removed_notional_rate:bid"] = removedBid / timeDelta
			out["removed_notional_rate:ask"] = removedAsk / timeDelta
			out["net_displayed_flow_rate:bid"] = netBid / timeDelta
			out["net_displayed_flow_rate:ask"] = netAsk / timeDelta
			out["book_turnover_rate"] = bookTurnoverRate
			out["net_book_change_rate"] = netBookChangeRate
			out["signed_net_displayed_flow_rate"] = signedNetFlowRate
		}
	}

	emitEstimate(out, "book_imbalance", &state.bookImbEstimator, bookImb)

	if state.hasPrevBookImb {
		out["book_imbalance_velocity"] = bookImb - state.prevBookImb
	}

	state.prevBookImb = bookImb
	state.hasPrevBookImb = true

	emitEstimate(out, "resolution_gap", &state.gapEstimator, gap)

	if state.hasPrevGap {
		out["resolution_gap_velocity"] = gap - state.prevGap
	}

	state.prevGap = gap
	state.hasPrevGap = true

	if hasRate {
		turnoverZ, hasTurnoverZ := emitEstimate(out, "turnover", &state.turnoverEstimator, bookTurnoverRate)

		if baseline, ok := out["turnover_baseline"]; ok && baseline > 0 {
			out["turnover_ratio"] = bookTurnoverRate / baseline
		}

		netChangeZ, hasNetChangeZ := emitEstimate(
			out, "net_book_change_rate", &state.netChangeEstimator, netBookChangeRate,
		)
		emitEstimate(
			out, "signed_net_displayed_flow_rate", &state.signedFlowEstimator, signedNetFlowRate,
		)

		if hasTurnoverZ && hasNetChangeZ {
			state.historyPath(out, [2]float64{turnoverZ, netChangeZ})
		}
	}

	state.prevBids = currentBids
	state.prevAsks = currentAsks
	state.prevFullBid = fullBid
	state.prevFullAsk = fullAsk
	state.prevBidWorst = bids[len(bids)-1][0]
	state.prevAskWorst = asks[len(asks)-1][0]
	state.prevTotal = total
	state.prevAtNano = atNano
	prevAt := state.prevAt
	hadPrev := state.hasPrev
	state.prevAt = prior.At
	state.hasPrev = true

	res := prior.Next(signal.Name(), out)

	// Flow is measured over (previous frame, this frame].
	if hadPrev {
		res.From = prevAt
	}

	return res
}

/*
emitEstimate writes name's baseline, divergence, and z-score as far as the
estimator defines them, and returns the z-score with its definedness.
*/
func emitEstimate(
	out map[string]float64, name string, estimator *causalEstimator, value float64,
) (float64, bool) {
	hasBaseline, baseline, residual, hasZ, zScore := estimator.Step(value)

	if hasBaseline {
		out[name+"_baseline"] = baseline
		out[name+"_divergence"] = residual
	}

	if hasZ {
		out[name+"_zscore"] = zScore
	}

	return zScore, hasZ
}

/*
levelFlow returns notional added and removed between two windows of one book
side. A price only in the previous window is removed unless the current side
is full and the price lies beyond its edge (it left the window, not the book);
a price only in the current window is added unless the previous side was full
and the price lies beyond that edge. inside reports whether price lies within
the window ending at edge.
*/
func levelFlow(
	prev, current map[float64]float64,
	prevFull, currentFull bool,
	prevEdge, currentEdge float64,
	inside func(price, edge float64) bool,
) (added, removed float64) {
	for price, notional := range current {
		before, seen := prev[price]

		if !seen {
			if !prevFull || inside(price, prevEdge) {
				added += notional
			}

			continue
		}

		if notional > before {
			added += notional - before
		}

		if notional < before {
			removed += before - notional
		}
	}

	for price, before := range prev {
		if _, held := current[price]; held {
			continue
		}

		if !currentFull || inside(price, currentEdge) {
			removed += before
		}
	}

	return added, removed
}

/*
historyPath scores how far this frame's (turnover, net-change) z-score point
lies from the nearest retained one, and that distance's rank among past
nearest distances.
*/
func (state *symbolState) historyPath(out map[string]float64, target [2]float64) {
	if len(state.historyPoints) > 0 {
		minDist := math.Inf(1)

		for _, point := range state.historyPoints {
			dx := target[0] - point[0]
			dy := target[1] - point[1]
			minDist = math.Min(minDist, math.Sqrt(dx*dx+dy*dy))
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
