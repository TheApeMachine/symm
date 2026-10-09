package pumpdump

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
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
the z-score only once the prior dispersion is positive.
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

/*
retainedQuantities bounds the prior trade-quantity distribution the volume
clock takes its median from, matching the retention of the other histories.
*/
const retainedQuantities = 256

/*
volumeBar is the open bar of the volume clock: its target Q* is fixed when it
opens, from the median of the prior retained trade quantities.
*/
type volumeBar struct {
	open       bool
	target     float64
	startNanos float64
	fromMid    float64
	quantity   float64
	notional   float64
	trades     float64
}

/*
completedBar is one closed volume bar.
*/
type completedBar struct {
	target, quantity, notional, trades float64
	startNanos, duration               float64
	fromMid, atMid                     float64
}

type volumeClock struct {
	quantities    []float64
	bar           volumeBar
	completedBars float64
	hasPrev       bool
	prevAtNanos   float64
}

/*
Step adds one trade to the clock and returns the bar it closed, if any.
Every trade counts, including trades sharing a timestamp; a bar only closes
once it has reached its target with positive elapsed duration. Without a prior
quantity distribution no bar opens: the first trade only seeds it.
*/
func (clock *volumeClock) Step(price, qty, atNanos, midpoint float64) (
	interval float64, hasInterval bool, closed completedBar, hasClosed bool,
) {
	if clock.hasPrev && atNanos > clock.prevAtNanos {
		interval = (atNanos - clock.prevAtNanos) / 1e9
		hasInterval = true
	}

	clock.hasPrev = true
	clock.prevAtNanos = atNanos

	if !clock.bar.open && len(clock.quantities) > 0 {
		clock.bar = volumeBar{
			open:       true,
			target:     median(clock.quantities),
			startNanos: atNanos,
			fromMid:    midpoint,
		}
	}

	if clock.bar.open {
		clock.bar.quantity += qty
		clock.bar.notional += price * qty
		clock.bar.trades++

		if clock.bar.quantity >= clock.bar.target && atNanos > clock.bar.startNanos {
			bar := clock.bar
			closed = completedBar{
				target:     bar.target,
				quantity:   bar.quantity,
				notional:   bar.notional,
				trades:     bar.trades,
				startNanos: bar.startNanos,
				duration:   (atNanos - bar.startNanos) / 1e9,
				fromMid:    bar.fromMid,
				atMid:      midpoint,
			}
			hasClosed = true
			clock.completedBars++

			// The next bar opens where this one closed, on the
			// distribution that now includes this trade.
			clock.quantities = append(clock.quantities, qty)
			clock.bar = volumeBar{
				open:       true,
				target:     median(clock.quantities),
				startNanos: atNanos,
				fromMid:    midpoint,
			}
			clock.retain()

			return interval, hasInterval, closed, hasClosed
		}
	}

	clock.quantities = append(clock.quantities, qty)
	clock.retain()

	return interval, hasInterval, closed, hasClosed
}

func (clock *volumeClock) retain() {
	if len(clock.quantities) > retainedQuantities {
		clock.quantities = clock.quantities[len(clock.quantities)-retainedQuantities:]
	}
}

func median(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2

	if len(sorted)%2 == 1 {
		return sorted[mid]
	}

	return (sorted[mid-1] + sorted[mid]) / 2
}

type symbolState struct {
	clock              volumeClock
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
		if book == nil {
			return
		}

		b := book.BestBid()
		a := book.BestAsk()

		if b == nil || a == nil || b.Price == nil || a.Price == nil ||
			b.Quantity == nil || a.Quantity == nil {
			return
		}

		bid = kraken.Float64(b.Price)
		ask = kraken.Float64(a.Price)
		hasBook = true
	})

	if hasBook {
		touch := map[string]float64{
			"bid": bid,
			"ask": ask,
		}

		for _, value := range touch {
			if !broker.ValidTouchValue(value) {
				signal.Error(broker.InvalidTouch("pumpdump", prior.Label, touch))
				return nil
			}
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

		if hasBaseline && baseline > 0 {
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

	interval, hasInterval, bar, closed := state.clock.Step(price, qty, atNanos, midpoint)
	out["completed_bars"] = state.clock.completedBars

	if hasInterval {
		out["trade_interval_seconds"] = interval
	}

	var notionalZ float64
	var hasNotionalZ bool

	if closed {
		notionalRate := bar.notional / bar.duration

		out["volume_bar_quantity"] = bar.quantity
		out["volume_bar_notional"] = bar.notional
		out["volume_bar_trade_count"] = bar.trades
		out["volume_bar_duration"] = bar.duration
		out["volume_rate"] = bar.quantity / bar.duration
		out["notional_rate"] = notionalRate
		out["trade_rate"] = bar.trades / bar.duration

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

		if bar.fromMid > 0 && bar.atMid > 0 {
			midReturn := math.Log(bar.atMid / bar.fromMid)

			out["midpoint:from"] = bar.fromMid
			out["midpoint:at"] = bar.atMid
			out["midpoint_log_return"] = midReturn
			out["midpoint_return_rate"] = midReturn / bar.duration
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
	}

	if hasSpreadZ && hasNotionalZ {
		state.historyPath(out, [2]float64{spreadZ, notionalZ})
	}

	res := prior.Next(signal.Name(), out)

	// A completed bar's metrics describe (bar open, this trade].
	if closed {
		res.From = time.Unix(0, int64(bar.startNanos)).UTC()
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
