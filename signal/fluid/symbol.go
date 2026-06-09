package fluid

import (
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

/*
FluidSymbol models one symbol's order book as a 1D reaction-diffusion fluid field.

Divergence is ∇·v at the touch. Viscosity is the near-touch replenishment
rate after consumption. Reynolds is |v|·L/ν with L equal to the bid-ask spread.
*/
type bufferedTrade struct {
	at    time.Time
	price float64
	qty   float64
	side  string
}

type FluidSymbol struct {
	symbol         string
	book           krakenmarket.Book
	bookReady      bool
	bookDiverged   bool
	divergedLogged bool
	bookDepth      int
	changePct      float64
	volume         float64
	last           float64
	bid            float64
	ask            float64
	spreadBPS      float64
	flux           *fluxAccumulator
	grid           *FluidGrid
	bufferedTrades []bufferedTrade
	lastEventAt    time.Time
}

type fluidReading struct {
	symbol        string
	price         float64
	spreadBPS     float64
	reynolds      float64
	divergence    float64
	viscosity     float64
	sourceBalance float64
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
		flux:      newFluxAccumulator(),
		grid:      grid,
	}, nil
}

/*
ConfigureTick rebuilds the price lattice from the exchange price increment before the first book snapshot.
*/
func (state *FluidSymbol) ConfigureTick(priceIncrement float64) error {
	if priceIncrement <= 0 {
		return fmt.Errorf("fluid: %s price increment must be positive", state.symbol)
	}

	if state.bookReady {
		return nil
	}

	if state.grid != nil && state.grid.tickSize == priceIncrement {
		return nil
	}

	halfWidth := viper.GetInt("signals.fluid.grid_half_width")

	if halfWidth <= 0 {
		return fmt.Errorf("fluid: signals.fluid.grid_half_width must be positive")
	}

	integrationInterval := viper.GetDuration("signals.fluid.integration_interval")

	if integrationInterval <= 0 {
		return fmt.Errorf("fluid: signals.fluid.integration_interval must be positive")
	}

	grid, gridErr := newFluidGrid(priceIncrement, halfWidth, integrationInterval)

	if gridErr != nil {
		return gridErr
	}

	state.grid = grid

	return nil
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

	if ingestErr := state.grid.ingestBook(state.book.Bids, state.book.Asks, mid, at); ingestErr != nil {
		return ingestErr
	}

	if flushErr := state.flushBufferedTrades(); flushErr != nil {
		return flushErr
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

func (state *FluidSymbol) FeedTrade(
	at time.Time,
	price, qty float64,
	side string,
) error {
	if !at.IsZero() {
		state.lastEventAt = at
	}

	if !state.bookReady || state.grid.lastMidPrice <= 0 {
		state.bufferTrade(at, price, qty, side)

		return nil
	}

	if state.grid.priceIndex(state.grid.lastMidPrice, price) < 0 {
		state.bufferTrade(at, price, qty, side)

		return nil
	}

	return state.applyTrade(at, price, qty, side)
}

func (state *FluidSymbol) bufferTrade(
	at time.Time,
	price, qty float64,
	side string,
) {
	state.bufferedTrades = append(state.bufferedTrades, bufferedTrade{
		at:    at,
		price: price,
		qty:   qty,
		side:  side,
	})
}

func (state *FluidSymbol) applyTrade(
	at time.Time,
	price, qty float64,
	_ string,
) error {
	if qty > 0 && state.flux.hasTarget() {
		if err := state.flux.addTrade(qty); err != nil {
			return err
		}
	}

	return state.grid.ingestTrade(price, qty, at)
}

func (state *FluidSymbol) flushBufferedTrades() error {
	if len(state.bufferedTrades) == 0 || state.grid.lastMidPrice <= 0 {
		return nil
	}

	pending := state.bufferedTrades
	state.bufferedTrades = nil

	for _, trade := range pending {
		if state.grid.priceIndex(state.grid.lastMidPrice, trade.price) < 0 {
			continue
		}

		if err := state.applyTrade(trade.at, trade.price, trade.qty, trade.side); err != nil {
			return err
		}
	}

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

	divergence := state.grid.midVelocityDivergence()
	viscosity := state.grid.viscosity()
	reynolds := state.grid.reynolds(spread)

	if math.IsNaN(reynolds) {
		return fluidReading{}, false
	}

	return fluidReading{
		symbol:        state.symbol,
		price:         state.last,
		spreadBPS:     state.spreadBPS,
		reynolds:      reynolds,
		divergence:    divergence,
		viscosity:     viscosity,
		sourceBalance: state.grid.midSourceBalance(),
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

	divergence := state.grid.midVelocityDivergence()
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
		"src_bal":    state.grid.midSourceBalance(),
	})
}
