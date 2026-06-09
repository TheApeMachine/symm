package fluid

import (
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
FluidSymbol models one symbol's order book as a 1D reaction-diffusion fluid field.

Divergence is ∇·(ρv) at the touch. Viscosity is the near-touch replenishment
rate after consumption. Reynolds is |v|·L/ν with L equal to the bid-ask spread.
*/
type FluidSymbol struct {
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
	grid           *FluidGrid
	lastEventAt    time.Time
}

type fluidReading struct {
	symbol     string
	price      float64
	spreadBPS  float64
	reynolds   float64
	divergence float64
	viscosity  float64
}

func NewFluidSymbol(symbol string) (*FluidSymbol, error) {
	depth := viper.GetInt("market.book_depth_levels")

	if depth <= 0 {
		return nil, fmt.Errorf("fluid: market.book_depth_levels must be positive")
	}

	grid, gridErr := NewFluidGrid()

	if gridErr != nil {
		return nil, gridErr
	}

	return &FluidSymbol{
		symbol:    symbol,
		bookDepth: depth,
		pressure:  adaptive.NewEMA(0),
		flux:      newFluxAccumulator(),
		grid:      grid,
	}, nil
}

func (state *FluidSymbol) FeedTicker(row krakenmarket.TickerUpdate, at time.Time) error {
	if !at.IsZero() {
		state.lastEventAt = at
	}

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
	}

	if row.Bid > 0 {
		state.bid = row.Bid
	}

	if row.Ask > 0 {
		state.ask = row.Ask
	}

	return nil
}

func (state *FluidSymbol) FeedBook(update krakenmarket.Book, at time.Time) error {
	return state.feedBookLocked(update, at)
}

func (state *FluidSymbol) feedBookLocked(update krakenmarket.Book, at time.Time) error {
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

	if at.IsZero() {
		return fmt.Errorf("fluid: book event time is zero")
	}

	state.lastEventAt = at

	flux := state.trustedSideChangeFlux(beforeBids, state.book.Bids, at) +
		state.trustedSideChangeFlux(beforeAsks, state.book.Asks, at)

	state.updateTouchLocked(state.book.Bids, state.book.Asks)

	mid := (state.bid + state.ask) / 2

	if stepErr := state.grid.step(state.book.Bids, state.book.Asks, mid, at); stepErr != nil {
		return stepErr
	}

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
	if !at.IsZero() {
		state.lastEventAt = at
	}

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

func (state *FluidSymbol) Diverged() bool {
	return state.bookDiverged
}

func (state *FluidSymbol) HasBook() bool {
	return state.bookReady
}

func (state *FluidSymbol) Row() map[string]any {
	if state.bookDiverged {
		return nil
	}

	if state.lastEventAt.IsZero() {
		return nil
	}

	return state.wireRowLocked()
}

func (state *FluidSymbol) Reading() (fluidReading, bool) {
	if state.bookDiverged || state.last <= 0 || state.lastEventAt.IsZero() {
		return fluidReading{}, false
	}

	if !state.grid.ready() {
		return fluidReading{}, false
	}

	spread := state.ask - state.bid

	if spread <= 0 {
		return fluidReading{}, false
	}

	divergence := state.grid.midMomentumDivergence()
	viscosity := state.grid.viscosity()
	reynolds := state.grid.reynolds(spread)

	if math.IsNaN(reynolds) {
		return fluidReading{}, false
	}

	return fluidReading{
		symbol:     state.symbol,
		price:      state.last,
		spreadBPS:  state.spreadBPS,
		reynolds:   reynolds,
		divergence: divergence,
		viscosity:  viscosity,
	}, true
}

func (state *FluidSymbol) wireRowLocked() map[string]any {
	if state.last <= 0 || !state.grid.ready() {
		return nil
	}

	spread := state.ask - state.bid

	if spread <= 0 {
		return nil
	}

	divergence := state.grid.midMomentumDivergence()
	viscosity := state.grid.viscosity()
	reynolds := state.grid.reynolds(spread)

	if math.IsNaN(reynolds) {
		return nil
	}

	return WireRow(map[string]any{
		"symbol":     state.symbol,
		"diverged":   state.bookDiverged,
		"change_pct": state.changePct,
		"vol":        state.volume,
		"div":        divergence,
		"visc":       viscosity,
		"re":         reynolds,
	})
}
