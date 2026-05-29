package fluid

import "time"

/*
fluxAccumulator measures book churn and fill volume over volume-clocked bars instead of
chronological windows.

A physical pipe has a constant diameter; an exchange's liquidity pool is a pipe made of vapour,
and ten seconds of wall-clock time holds wildly different amounts of information on a dead Sunday
night versus a frenzied sell-off. Sampling the fill-to-cancel ratio on a fixed clock therefore
confuses an empty market with a viscous one. Here the bar instead closes once a target amount of
base volume has actually traded, so every bar carries a consistent quantum of activity. A wall-clock
maxAge still force-closes a bar so a long quiet stretch cannot accumulate unbounded churn, and when
the per-symbol volume target is unknown the accumulator falls back to pure time bars — the previous
behaviour.

The last completed bar is what callers read, so the fill-to-cancel ratio is always measured over a
whole, consistent bucket rather than whatever fraction has accrued since the last roll.
*/
type fluxAccumulator struct {
	maxAge      time.Duration
	target      float64
	start       time.Time
	started     bool
	progress    float64
	bookOpen    float64
	tradeOpen   float64
	bookClosed  float64
	tradeClosed float64
	haveClosed  bool
}

func newFluxAccumulator(maxAge time.Duration) *fluxAccumulator {
	return &fluxAccumulator{maxAge: maxAge}
}

/*
setTarget sets the base volume that closes one bar. A non-positive target disables volume rolling
and the accumulator behaves as a pure time bar of width maxAge.
*/
func (flux *fluxAccumulator) setTarget(target float64) {
	if target < 0 {
		target = 0
	}

	flux.target = target
}

/*
addBook folds one book-churn reading into the open bar. Book updates carry no traded volume, so
they can only trigger the wall-clock fallback roll.
*/
func (flux *fluxAccumulator) addBook(at time.Time, churn float64) {
	if churn <= 0 {
		return
	}

	flux.startOrAgeRoll(at)
	flux.bookOpen += churn
}

/*
addTrade folds one fill into the open bar and advances the volume clock. The fill that crosses the
target completes the current bar — it is counted in that bar — and a fresh, empty bar opens.
*/
func (flux *fluxAccumulator) addTrade(at time.Time, qty float64) {
	if qty <= 0 {
		return
	}

	flux.startOrAgeRoll(at)
	flux.tradeOpen += qty
	flux.progress += qty

	if flux.target > 0 && flux.progress >= flux.target {
		flux.close(at)
	}
}

// startOrAgeRoll seeds the first bar and force-closes a stale one on the wall-clock fallback
// before the triggering event is folded — so an event arriving after the bar has aged out opens
// the next bar rather than landing in the expired one.
func (flux *fluxAccumulator) startOrAgeRoll(at time.Time) {
	if !flux.started {
		flux.started = true
		flux.start = at

		return
	}

	if flux.maxAge > 0 && at.Sub(flux.start) >= flux.maxAge {
		flux.close(at)
	}
}

func (flux *fluxAccumulator) close(at time.Time) {
	flux.bookClosed = flux.bookOpen
	flux.tradeClosed = flux.tradeOpen
	flux.haveClosed = true
	flux.bookOpen = 0
	flux.tradeOpen = 0
	flux.progress = 0
	flux.start = at
}

/*
bookFlux returns the book churn of the last completed bar, or the open bar before any has closed.
*/
func (flux *fluxAccumulator) bookFlux() float64 {
	if flux.haveClosed {
		return flux.bookClosed
	}

	return flux.bookOpen
}

/*
tradeFlux returns the fill volume of the last completed bar, or the open bar before any has closed.
*/
func (flux *fluxAccumulator) tradeFlux() float64 {
	if flux.haveClosed {
		return flux.tradeClosed
	}

	return flux.tradeOpen
}
