package fluid

import (
	"math"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
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
	mu           sync.RWMutex
	symbol       string
	book         market.Book
	bookReady    bool
	bookDiverged bool
	bookDepth    int
	buyPressure  float64
	changePct    float64
	volume       float64
	last         float64
	bid          float64
	ask          float64
	pressure     *adaptive.EMA
	spreadBPS    float64
	flux         *fluxAccumulator
	priceFD      *adaptive.FracDiff
	fracScale    adaptive.AlphaEMA
	fracReturn   float64
	tracked      *perspectives.Category
}

// fracScaleAlpha smooths the running magnitude of the fractional price return,
// the baseline against which turbulence is measured as an excess over the norm.
const fracScaleAlpha = 0.05

func NewFluidSymbol(symbol string) *FluidSymbol {
	depth := viper.GetViper().GetInt("market.book_depth_levels")

	if depth <= 0 {
		depth = 10
	}

	return &FluidSymbol{
		symbol:    symbol,
		bookDepth: depth,
		pressure:  adaptive.NewEMA(0),
		flux:      newFluxAccumulator(viper.GetViper().GetDuration("signals.book_flux_window")),
		priceFD: adaptive.NewFracDiff(
			viper.GetViper().GetFloat64("signals.fractional_diff_order"),
			viper.GetViper().GetInt("signals.fractional_diff_width"),
		),
		tracked: perspectives.NewCategory(perspectives.CategoryTypeNone),
	}
}

func (state *FluidSymbol) FeedTicker(row market.TickerUpdate) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.changePct = row.ChangePct
	state.volume = row.Volume

	barsPerDay := viper.GetFloat64("signals.volume_clock_bars_per_day")

	if row.Volume > 0 && barsPerDay > 0 {
		state.flux.setTarget(row.Volume / barsPerDay)
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

func (state *FluidSymbol) FeedBook(update market.Book) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.feedBookLocked(update)
}

func (state *FluidSymbol) feedBookLocked(update market.Book) {
	if update.IsSnapshot() {
		state.bookReady = true
		state.bookDiverged = false
	} else if !state.bookReady {
		return
	}

	beforeBids := state.cloneBookLevels(state.book.Bids)
	beforeAsks := state.cloneBookLevels(state.book.Asks)

	state.book.Fold(update, state.bookDepth)
	state.verifyBookChecksumLocked(update.Checksum)

	flux := state.sideChangeFlux(beforeBids, state.book.Bids) + state.sideChangeFlux(beforeAsks, state.book.Asks)

	state.updateTouchLocked(state.book.Bids, state.book.Asks)

	if flux <= 0 {
		return
	}

	state.flux.addBook(time.Now(), flux)
}

func (state *FluidSymbol) verifyBookChecksumLocked(expected int64) {
	if expected == 0 {
		return
	}

	if state.book.ComputedChecksum() != expected {
		state.bookDiverged = true
	}
}

func (state *FluidSymbol) cloneBookLevels(levels []market.BookLevel) []market.BookLevel {
	if len(levels) == 0 {
		return nil
	}

	out := make([]market.BookLevel, len(levels))
	copy(out, levels)

	return out
}

func (state *FluidSymbol) updateTouchLocked(bids, asks []market.BookLevel) {
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

func (state *FluidSymbol) sideChangeFlux(previous, updated []market.BookLevel) float64 {
	previousByPrice := make(map[float64]float64, len(previous))

	for _, level := range previous {
		previousByPrice[level.Price] = level.Qty
	}

	flux := 0.0
	seen := make(map[float64]bool, len(updated))

	for _, level := range updated {
		flux += math.Abs(level.Qty - previousByPrice[level.Price])
		seen[level.Price] = true
	}

	for price, qty := range previousByPrice {
		if seen[price] {
			continue
		}

		flux += qty
	}

	return flux
}

func (state *FluidSymbol) FeedTradeSide(at time.Time, qty float64, side string) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if qty > 0 {
		state.flux.addTrade(at, qty)
	}

	sign := -1.0

	if side == "buy" {
		sign = 1.0
	}

	state.buyPressure = errnie.Does(func() (float64, error) {
		return state.pressure.Next(0, sign)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()
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

func (state *FluidSymbol) Measure() (perspectives.Measurement, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.bookDiverged {
		return perspectives.Measurement{}, false
	}

	row := state.wireRowLocked()

	if row == nil {
		return perspectives.Measurement{}, false
	}

	re, ok := row["re"].(float64)

	if !ok || re <= 0 {
		return perspectives.Measurement{}, false
	}

	divergence, _ := row["div"].(float64)
	turbulence, _ := row["turb_fd"].(float64)
	viscosity, _ := row["visc"].(float64)
	category, evidence := fluidReading(divergence, turbulence, viscosity, re)

	confidence, err := state.tracked.Observe(category, evidence)

	if err != nil {
		errnie.Error(err)

		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Symbol:     state.symbol,
		Source:     perspectives.SourceFluid,
		Category:   category,
		Last:       state.last,
		SpreadBPS:  state.spreadBPS,
		Strength:   re,
		Confidence: confidence,
	}, true
}

func (state *FluidSymbol) wireRowLocked() map[string]any {
	bids := state.book.Bids
	asks := state.book.Asks
	imbalance := 0.0
	pressure := (state.buyPressure + 1) / 2
	visc := 1 / (1 + state.spreadBPS/100)

	if len(bids) > 0 && len(asks) > 0 {
		bidVolume := 0.0
		askVolume := 0.0

		for _, level := range bids {
			bidVolume += level.Qty
		}

		for _, level := range asks {
			askVolume += level.Qty
		}

		total := bidVolume + askVolume

		if total > 0 {
			imbalance = (bidVolume - askVolume) / total
		}
	}

	if state.volume <= 0 && state.changePct == 0 && imbalance == 0 && pressure == 0.5 {
		return nil
	}

	vort := state.buyPressure

	if tradeVol := state.flux.tradeFlux(); tradeVol > 0 {
		vort = state.buyPressure * (1 + state.flux.bookFlux()/tradeVol)
	}

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
