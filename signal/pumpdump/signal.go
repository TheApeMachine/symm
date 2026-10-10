package pumpdump

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/volumeclock"
)

type causalEstimator struct {
	count float64
	mean  float64
	m2    float64
}

/*
Step scores value against the estimator's state before it, then incorporates
it. The baseline (and residual against it) is defined from the second sample;
the z-score only once core.PriorScale admits the prior dispersion
(enough prior samples, a scale not negligible next to the values).
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

	scale, scorable := core.PriorScale(priorCount, priorM2, value, priorMean)

	if !scorable {
		return true, priorMean, residual, false, 0
	}

	return true, priorMean, residual, true, residual / scale
}

type symbolState struct {
	clock              volumeclock.Clock
	notionalEstimator  causalEstimator
	spreadEstimator    causalEstimator
	midReturnEstimator causalEstimator
	hasPrevNotional    bool
	prevNotionalRate   float64
	hasPrevSpreadDiv   bool
	prevSpreadDiv      float64
	hasPrevMidReturn   bool
	prevMidReturn      float64
	historyPoints      [][2]float64
	historyDistances   []float64
}

type Signal struct {
	*runtime.System
	books  broker.BookSource
	mu     sync.Mutex
	states map[string]*symbolState
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books:  books,
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "pumpdump", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[pumpdump] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[pumpdump] book manager is required", nil))
		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	if priceEntry == nil || priceEntry.Metric == nil {
		signal.Error(errnie.Err(errnie.Validation, "[pumpdump] price is required", nil))
		return nil
	}
	price := priceEntry.Metric.Raw

	qtyEntry := data.Pull(prior.Read("qty"))
	if qtyEntry == nil || qtyEntry.Metric == nil || qtyEntry.Metric.Raw <= 0 {
		return nil
	}
	qty := qtyEntry.Metric.Raw

	var bid, ask float64
	var hasBook bool

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil || book.BestBid() == nil || book.BestAsk() == nil {
			return
		}

		bid = book.BestBid().Price.Float64()
		ask = book.BestAsk().Price.Float64()
		hasBook = true
	})

	if hasBook {
		if bid <= 0 || ask <= 0 || math.IsNaN(bid) || math.IsNaN(ask) || math.IsInf(bid, 0) || math.IsInf(ask, 0) {
			signal.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf("[%s] %s: non-finite or non-positive book touch: bid=%v ask=%v", signal.Name(), prior.Label, bid, ask),
				nil,
			))
			return nil
		}

		if ask <= bid {
			errnie.Warn(fmt.Sprintf("[pumpdump] dropping frame for %s with crossed book: bid=%v ask=%v", prior.Label, bid, ask))
			return nil
		}
	}

	atNanos := float64(prior.At.UnixNano())
	hasTouch := bid > 0 && ask > 0

	out := map[string]float64{
		"trade_price":    price,
		"trade_quantity": qty,
		"trade_notional": price * qty,
	}

	midpoint := 0.0

	signal.mu.Lock()
	defer signal.mu.Unlock()

	state, found := signal.states[prior.Label]

	if !found {
		state = &symbolState{}
		signal.states[prior.Label] = state
	}

	var spreadZ float64
	var hasSpreadZ bool

	if hasTouch {
		midpoint = (bid + ask) * 0.5
		spread := ask - bid

		out["best_bid"] = bid
		out["best_ask"] = ask
		out["midpoint"] = midpoint
		out["spread"] = spread
		out["relative_spread"] = spread / midpoint

		hasBaseline, baseline, _, hasZ, zScore := state.spreadEstimator.Step(spread)

		if hasBaseline && baseline > 0 && !core.Negligible(baseline, spread) {
			divergence := math.Log(spread / baseline)
			out["spread_ratio"] = spread / baseline
			out["spread_divergence"] = divergence
			out["relative_spread_baseline"] = baseline / midpoint

			if state.hasPrevSpreadDiv {
				out["spread_divergence_velocity"] = divergence - state.prevSpreadDiv
			}

			state.prevSpreadDiv = divergence
			state.hasPrevSpreadDiv = true
		}

		if hasZ {
			out["spread_zscore"] = zScore
			spreadZ, hasSpreadZ = zScore, true
		}
	}

	tick := state.clock.Step(price, qty, atNanos, midpoint)
	interval, hasInterval, bar, closed := tick.Interval, tick.HasInterval, tick.Bar, tick.Closed
	out["completed_bars"] = state.clock.CompletedBars

	if hasInterval {
		out["trade_interval_seconds"] = interval
	}

	var notionalZ float64
	var hasNotionalZ bool

	// A bar's rates need a duration the venue clock resolves
	// (core.Resolvable); a bar closed within that grain leaves them undefined.
	if closed {
		out["volume_bar_quantity"] = bar.Quantity
		out["volume_bar_notional"] = bar.Notional
		out["volume_bar_trade_count"] = bar.Trades
		out["volume_bar_duration"] = bar.Duration
	}

	if closed && core.Resolvable(bar.Duration) {
		notionalRate := bar.Notional / bar.Duration

		out["volume_rate"] = bar.Quantity / bar.Duration
		out["notional_rate"] = notionalRate
		out["trade_rate"] = bar.Trades / bar.Duration

		// Positive rates are modeled multiplicatively: the baseline is the
		// causal mean of the log rate.
		hasBaseline, logBaseline, logResidual, hasZ, zScore := state.notionalEstimator.Step(math.Log(notionalRate))

		if hasBaseline {
			out["notional_rate_baseline"] = math.Exp(logBaseline)
			out["notional_rate_ratio"] = math.Exp(logResidual)
			out["notional_rate_divergence"] = logResidual
		}

		if hasZ {
			out["notional_rate_zscore"] = zScore
			notionalZ, hasNotionalZ = zScore, true
		}

		if state.hasPrevNotional {
			out["notional_rate_velocity"] = notionalRate - state.prevNotionalRate
		}

		state.prevNotionalRate = notionalRate
		state.hasPrevNotional = true
	}

	if closed && bar.FromMid > 0 && bar.AtMid > 0 {
		midReturn := math.Log(bar.AtMid / bar.FromMid)

		out["midpoint:from"] = bar.FromMid
		out["midpoint:at"] = bar.AtMid
		out["midpoint_log_return"] = midReturn

		if core.Resolvable(bar.Duration) {
			out["midpoint_return_rate"] = midReturn / bar.Duration
		}

		out["positive_midpoint_return"] = math.Max(midReturn, 0)
		out["negative_midpoint_return"] = math.Max(-midReturn, 0)

		hasMidBaseline, midBaseline, midResidual, hasMidZ, midZ := state.midReturnEstimator.Step(midReturn)

		if hasMidBaseline {
			out["midpoint_return_baseline"] = midBaseline
			out["midpoint_return_divergence"] = midResidual
		}

		if hasMidZ {
			out["midpoint_return_zscore"] = midZ
		}

		if state.hasPrevMidReturn {
			out["midpoint_return_velocity"] = midReturn - state.prevMidReturn
		}

		state.prevMidReturn = midReturn
		state.hasPrevMidReturn = true
	}

	if hasSpreadZ && hasNotionalZ {
		state.historyPath(out, [2]float64{spreadZ, notionalZ})
	}

	res := prior.Next(signal.Name(), out)

	// A completed bar's metrics describe (bar open, this trade].
	if closed {
		res.From = time.Unix(0, int64(bar.StartNanos)).UTC()
	}

	return res
}

/*
historyPath scores how far this frame's (spread, notional-rate) z-score point
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
