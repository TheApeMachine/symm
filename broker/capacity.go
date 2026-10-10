package broker

import (
	"math"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Depth is the book history position sizing reads: the version at a time, every
version over a window, and the newest one.
*/
type Depth interface {
	BookHistory
	BookWindow(symbol string, from, to time.Time, read func(*BookView)) bool
	Latest(symbol string, read func(*BookView))
}

/*
Capacity is how much base quantity one side of a book version absorbs while
the volume-weighted fill price stays within a slippage budget of the
midpoint. Bounded marks a lower bound: the side ran out of displayed levels
inside the budget while holding its full configured depth, so deeper levels
exist that the version does not show. Defined is false for a version without
both sides or with a crossed touch.
*/
type Capacity struct {
	Quantity  float64
	Reference float64
	At        time.Time
	Bounded   bool
	Defined   bool
}

/*
ExitCapacity is the quantity a market sell can take from the bids while its
VWAP stays at or above mid*(1-budget).
*/
func ExitCapacity(view *BookView, budget float64) Capacity {
	if view == nil {
		return Capacity{}
	}

	return walkCapacity(view, view.Bids, view.FullBid, budget, -1)
}

/*
EntryCapacity is the quantity a market buy can take from the asks while its
VWAP stays at or below mid*(1+budget).
*/
func EntryCapacity(view *BookView, budget float64) Capacity {
	if view == nil {
		return Capacity{}
	}

	return walkCapacity(view, view.Asks, view.FullAsk, budget, 1)
}

/*
walkCapacity takes whole levels best first while the running VWAP stays
within the limit, then the exact part of the next level that brings the VWAP
onto the limit: (gross + p*x) / (qty + x) = limit.
*/
func walkCapacity(view *BookView, levels []BookLevel, full bool, budget, sign float64) Capacity {
	if len(view.Bids) == 0 || len(view.Asks) == 0 || budget < 0 || math.IsNaN(budget) {
		return Capacity{}
	}

	bid, ask := view.Bids[0].Price, view.Asks[0].Price

	if bid <= 0 || bid >= ask {
		return Capacity{}
	}

	mid := (bid + ask) / 2
	limit := mid * (1 + sign*budget)
	qty, gross := 0.0, 0.0

	for _, level := range levels {
		nextQty := qty + level.Quantity
		nextGross := gross + level.Price*level.Quantity

		if sign*(nextGross-limit*nextQty) <= 0 {
			qty, gross = nextQty, nextGross
			continue
		}

		if part := (limit*qty - gross) / (level.Price - limit); part > 0 {
			qty += part
		}

		return Capacity{Quantity: qty, Reference: mid, At: view.At, Defined: true}
	}

	return Capacity{Quantity: qty, Reference: mid, At: view.At, Bounded: full, Defined: true}
}

/*
SlippageBudget is the per-side slippage the expected edge pays for: half of
what the median matched gain leaves after the round-trip taker fee. Without
matched gains it is one taker fee, reported as the fallback source. An edge
that does not clear the round-trip fee leaves no budget and is an error.
*/
func SlippageBudget(fee float64, gains []float64) (float64, string, error) {
	if len(gains) == 0 {
		return fee, BudgetFeeFallback, nil
	}

	budget := (median(gains) - 2*fee) / 2

	if budget <= 0 {
		return 0, BudgetMatchedEdge, errnie.Err(
			errnie.NotAcceptable, "[capacity] expected edge does not clear the round-trip fee", nil,
		)
	}

	return budget, BudgetMatchedEdge, nil
}

/*
Budget sources.
*/
const (
	BudgetMatchedEdge = "matched_edge"
	BudgetFeeFallback = "fee_fallback"
)

/*
ExpectedHold is the median B->C duration of the matched excursions. ok is
false without a positive one: the hold sizes both the exit-capacity window and
the participation limit, and nothing but matched statistics says how long a
move will take, so an entry without them is refused rather than sized from
how long the live path took to match.
*/
func ExpectedHold(holds []time.Duration) (time.Duration, bool) {
	positive := make([]float64, 0, len(holds))

	for _, hold := range holds {
		if hold > 0 {
			positive = append(positive, float64(hold))
		}
	}

	if len(positive) == 0 {
		return 0, false
	}

	return time.Duration(median(positive)), true
}

/*
Edge is what the matched stored excursions say about a live entry: the gain
c/b-1 and B->C duration of each readable one.
*/
type Edge struct {
	Gains []float64
	Holds []time.Duration
}

func median(values []float64) float64 {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	mid := len(sorted) / 2

	if len(sorted)%2 == 1 {
		return sorted[mid]
	}

	return (sorted[mid-1] + sorted[mid]) / 2
}

/*
Flow records each symbol's signed traded volume (taker buys positive) so a
position can be sized within the noise of net flow: Noise is the standard
deviation of signed volume summed over consecutive windows of one expected
hold. Every trade since the first is retained for the session.
*/
type Flow struct {
	mu     sync.Mutex
	trades map[string][]signedTrade
}

type signedTrade struct {
	at  time.Time
	qty float64
}

func NewFlow() *Flow {
	return &Flow{trades: make(map[string][]signedTrade)}
}

/*
Record adds one trade; side is the taker side.
*/
func (flow *Flow) Record(symbol string, at time.Time, side string, qty float64) {
	if flow == nil || at.IsZero() || qty <= 0 {
		return
	}

	if side == string(SELL) {
		qty = -qty
	}

	flow.mu.Lock()
	defer flow.mu.Unlock()

	trades := flow.trades[symbol]

	if count := len(trades); count > 0 && at.Before(trades[count-1].at) {
		at = trades[count-1].at
	}

	flow.trades[symbol] = append(trades, signedTrade{at: at, qty: qty})
}

/*
Noise is the sample standard deviation of signed volume over the complete
windows (now-(k+1)*hold, now-k*hold] observed since the first trade. ok is
false with fewer than two complete windows: one window has no dispersion.
*/
func (flow *Flow) Noise(symbol string, now time.Time, hold time.Duration) (float64, int, bool) {
	if flow == nil || hold <= 0 {
		return 0, 0, false
	}

	flow.mu.Lock()
	defer flow.mu.Unlock()

	trades := flow.trades[symbol]

	if len(trades) == 0 {
		return 0, 0, false
	}

	windows := int(now.Sub(trades[0].at) / hold)

	if windows < 2 {
		return 0, windows, false
	}

	sums := make([]float64, windows)

	for _, trade := range trades {
		if trade.at.After(now) {
			break
		}

		// Window k is (now-(k+1)*hold, now-k*hold]: now-at in [k, k+1) holds.
		if k := int(now.Sub(trade.at) / hold); k < windows {
			sums[k] += trade.qty
		}
	}

	mean := 0.0

	for _, sum := range sums {
		mean += sum
	}

	mean /= float64(windows)
	variance := 0.0

	for _, sum := range sums {
		variance += (sum - mean) * (sum - mean)
	}

	return math.Sqrt(variance / float64(windows-1)), windows, true
}
