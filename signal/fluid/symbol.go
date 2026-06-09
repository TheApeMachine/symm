package fluid

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
FluidSymbol models one symbol's order book as a fluid field — divergence
(imbalance), vorticity (flow), turbulence (stationary price velocity excess),
viscosity (spread), and a Reynolds number combining them — and maps that onto
the mechanical perspective. Confidence is classification clarity — margin to the
nearest category boundary; SNR is how surprising that clarity is versus the
symbol's own recent baseline, not the Reynolds number itself.

The book is a maintained market.Book. Liquidity flux — the field's vorticity input —
is measured as the change between the local book before and after each frame is folded
in, not between consecutive raw deltas, so it reflects genuine book churn rather than
the size of whatever slice the feed happened to send.
*/
type FluidSymbol struct {
	mu             sync.RWMutex
	symbol         string
	book           krakenmarket.Book
	bookReady      bool
	bookDiverged   bool
	divergedLogged bool
	bookDepth      int
	buyPressure    float64
	changePct      float64
	volume         float64
	last           float64
	bid            float64
	ask            float64
	pressure       *adaptive.EMA
	spreadBPS      float64
	flux           *fluxAccumulator
	priceFD        *adaptive.FracDiff
	fracScale      adaptive.AlphaEMA
	fracReturn     float64
}

type fluidReading struct {
	symbol     string
	price      float64
	spreadBPS  float64
	reynolds   float64
	divergence float64
	vorticity  float64
	turbulence float64
	viscosity  float64
}

var fluidDefaultBandEdges = []float64{0.2, 0.5, 1.5}

// fracScaleAlpha smooths the running magnitude of the fractional price return,
// the baseline against which turbulence is measured as an excess over the norm.
const fracScaleAlpha = 0.05

func NewFluidSymbol(symbol string) (*FluidSymbol, error) {
	depth := viper.GetInt("market.book_depth_levels")

	if depth <= 0 {
		depth = 10
	}

	state := &FluidSymbol{
		symbol:    symbol,
		bookDepth: depth,
		pressure:  adaptive.NewEMA(0),
		flux:      newFluxAccumulator(),
		priceFD: adaptive.NewFracDiff(
			viper.GetFloat64("signals.fractional_diff_order"),
			viper.GetInt("signals.fractional_diff_width"),
		),
	}

	return state, nil
}

func (state *FluidSymbol) FeedTicker(row krakenmarket.TickerUpdate) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.changePct = row.ChangePct
	state.volume = row.Volume

	if row.Volume > 0 {
		barsPerDay := viper.GetFloat64("signals.volume_clock_bars_per_day")

		if barsPerDay <= 0 {
			return errnie.Error(fmt.Errorf("fluid: signals.volume_clock_bars_per_day must be positive"))
		}

		if err := state.flux.setTarget(row.Volume / barsPerDay); err != nil {
			return err
		}
	}

	if row.Last > 0 {
		state.last = row.Last
		state.observePriceLocked(row.Last)
	}

	if row.Bid > 0 {
		state.bid = row.Bid
	}

	if row.Ask > 0 {
		state.ask = row.Ask
	}

	return nil
}

func (state *FluidSymbol) observePriceLocked(price float64) {
	if price <= 0 {
		return
	}

	value, ok := state.priceFD.Push(math.Log(price))

	if !ok {
		return
	}

	state.fracReturn = value
	_ = state.fracScale.Update(math.Abs(value), fracScaleAlpha)
}

func (state *FluidSymbol) FeedBook(update krakenmarket.Book) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	return state.feedBookLocked(update)
}

func (state *FluidSymbol) feedBookLocked(update krakenmarket.Book) error {
	if update.IsSnapshot() {
		state.bookReady = true
		state.bookDiverged = false
	} else if !state.bookReady {
		return nil
	}

	beforeBids := state.cloneBookLevels(state.book.Bids)
	beforeAsks := state.cloneBookLevels(state.book.Asks)

	state.book.Fold(update, state.bookDepth)
	state.verifyBookChecksumLocked(update.Checksum)

	at := time.Now()
	flux := state.trustedSideChangeFlux(beforeBids, state.book.Bids, at) +
		state.trustedSideChangeFlux(beforeAsks, state.book.Asks, at)

	state.updateTouchLocked(state.book.Bids, state.book.Asks)

	if flux <= 0 || !state.flux.hasTarget() {
		return nil
	}

	return state.flux.addBook(flux)
}

func (state *FluidSymbol) verifyBookChecksumLocked(expected int64) {
	if expected == 0 {
		return
	}

	if state.book.ComputedChecksum() == expected {
		state.bookDiverged = false
		state.divergedLogged = false

		return
	}

	state.bookDiverged = true

	// Divergence is telemetry, not a death sentence. On a drop-oldest bus a
	// missed delta makes a checksum mismatch inevitable within minutes.
	// Kraken only resends a snapshot on resubscribe; the fluid system requests
	// a per-symbol book resync (unsubscribe + resubscribe) which clears this.
	if !state.divergedLogged {
		state.divergedLogged = true
		errnie.Warn("fluid: book checksum diverged for " + state.symbol + " — field degraded, continuing")
	}
}

func (state *FluidSymbol) cloneBookLevels(levels []krakenmarket.BookLevel) []krakenmarket.BookLevel {
	if len(levels) == 0 {
		return nil
	}

	out := make([]krakenmarket.BookLevel, len(levels))
	copy(out, levels)

	return out
}

func (state *FluidSymbol) updateTouchLocked(bids, asks []krakenmarket.BookLevel) {
	if len(bids) == 0 || len(asks) == 0 {
		return
	}

	bid := bids[0].Price
	ask := asks[0].Price
	mid := (bid + ask) / 2

	state.bid = bid
	state.ask = ask

	if state.last <= 0 && mid > 0 {
		state.last = mid
	}

	if mid > 0 {
		state.spreadBPS = (ask - bid) / mid * 10000
	}
}

func (state *FluidSymbol) FeedTradeSide(at time.Time, qty float64, side string) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	if qty > 0 && state.flux.hasTarget() {
		if err := state.flux.addTrade(qty); err != nil {
			return err
		}
	}

	sign := -1.0

	if side == "buy" {
		sign = 1.0
	}

	value, err := state.pressure.Next(0, sign)

	if err != nil {
		return err
	}

	state.buyPressure = value

	return nil
}

// Diverged reports whether the maintained book disagrees with the exchange
// checksum — the trigger for a per-symbol book resync.
func (state *FluidSymbol) Diverged() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.bookDiverged
}

func (state *FluidSymbol) HasBook() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.bookReady
}

/*
Row returns the symbol's current fluid-field reading in the dashboard wire shape
(symbol, change_pct, vol, div, vort, turb, visc, re), or nil when the field has
no data yet.
*/
func (state *FluidSymbol) Row() map[string]any {
	state.mu.RLock()
	defer state.mu.RUnlock()

	if state.bookDiverged {
		return nil
	}

	return state.wireRowLocked()
}

func (state *FluidSymbol) Reading() (fluidReading, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.bookDiverged || state.last <= 0 {
		return fluidReading{}, false
	}

	row := state.wireRowLocked()

	if row == nil {
		return fluidReading{}, false
	}

	re, _ := row["re"].(float64)
	divergence, _ := row["div"].(float64)
	vorticity, _ := row["vort"].(float64)
	turbulence, _ := row["turb_fd"].(float64)
	viscosity, _ := row["visc"].(float64)

	return fluidReading{
		symbol:     state.symbol,
		price:      state.last,
		spreadBPS:  state.spreadBPS,
		reynolds:   re,
		divergence: divergence,
		vorticity:  vorticity,
		turbulence: turbulence,
		viscosity:  viscosity,
	}, true
}

func (state *FluidSymbol) wireRowLocked() map[string]any {
	if state.last <= 0 {
		return nil
	}

	bids := state.book.Bids
	asks := state.book.Asks
	imbalance := 0.0
	pressure := (state.buyPressure + 1) / 2
	visc := 1 / (1 + state.spreadBPS/100)

	if trusted, ok := state.trustedImbalanceLocked(bids, asks, time.Now()); ok {
		imbalance = trusted
	}

	if state.volume <= 0 && state.changePct == 0 && imbalance == 0 && pressure == 0.5 {
		return nil
	}

	vort := state.vorticityLocked(time.Now())

	turbulence := 0.0
	fracScale := state.fracScale.Value()

	if fracScale > 0 {
		turbulence = math.Max(0, math.Abs(state.fracReturn)/fracScale-1)
	}

	re := math.Max(
		math.Max(math.Abs(imbalance), math.Abs(vort)),
		turbulence,
	)

	return WireRow(map[string]any{
		"symbol":     state.symbol,
		"diverged":   state.bookDiverged,
		"change_pct": state.changePct,
		"vol":        state.volume,
		"div":        imbalance,
		"vort":       vort,
		"turb":       pressure * state.spreadBPS / 100,
		"turb_fd":    turbulence,
		"fd_ret":     state.fracReturn,
		"visc":       visc,
		"re":         re,
	})
}
