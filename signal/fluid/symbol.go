package fluid

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/symm/kraken"
)

/*
FluidSymbol models one symbol's order book as a 1D reaction-diffusion fluid field.

Divergence is ∇·(ρv) at the touch. Viscosity is the near-touch replenishment
rate after consumption. Reynolds is |v|·L/ν with L equal to the bid-ask spread.
*/
type bufferedTrade struct {
	at    time.Time
	price float64
	qty   float64
	side  string
}

/*
FluidSymbol owns one market's fluid grid and observed context so state
transitions remain symbol-local.
*/
type FluidSymbol struct {
	symbol             string
	book               *kraken.BookData
	bookReady          bool
	changePct          float64
	volume             float64
	last               float64
	bid                float64
	ask                float64
	spreadBPS          float64
	flux               *fluxAccumulator
	grid               *FluidGrid
	bufferedTrades     []bufferedTrade
	lastEventAt        time.Time
	dynamics           fluidDynamics
	config             symbolConfig
	instrumentTickSize float64
	memorySamples      []float64
}

/*
fluidReading captures one coherent fluid state for classification so
measurements come from a single epoch.
*/
type fluidReading struct {
	symbol            string
	price             float64
	spreadBPS         float64
	volume            float64
	changePct         float64
	reynolds          float64
	divergence        float64
	viscosity         float64
	velocityCurvature float64
	turbulence        float64
	sourceBalance     float64
	memory            float64
	midAddRate        float64
	midExecuteRate    float64
	dynamics          fluidDynamics
	gridSteps         int
}

/*
setInstrumentTickSize records validated instrument tick size so grid
configuration uses exchange metadata.
*/
func (state *FluidSymbol) setInstrumentTickSize(priceIncrement float64) {
	if priceIncrement <= 0 {
		return
	}

	state.instrumentTickSize = priceIncrement

	if state.grid != nil || !state.bookReady {
		return
	}

	_ = state.configureTickFromBook(state.book.Bids, state.book.Asks)
}

/*
configureTickFromBook derives tick configuration from a book update when the
exchange supplies its price increment.
*/
func (state *FluidSymbol) configureTickFromBook(
	bids, asks []kraken.BookLevel,
) error {
	bidPrices := make([]float64, len(bids))
	askPrices := make([]float64, len(asks))

	for index, level := range bids {
		bidPrices[index] = level.Price.Float64()
	}

	for index, level := range asks {
		askPrices[index] = level.Price.Float64()
	}

	fallback := state.config.tickSizeFallback

	if fallback <= 0 && state.instrumentTickSize > 0 {
		fallback = state.instrumentTickSize
	}

	tickSize, err := resolveBookTickSize(
		bidPrices,
		askPrices,
		state.instrumentTickSize,
		fallback,
	)

	if err != nil {
		if state.grid == nil {
			return nil
		}

		return fmt.Errorf("fluid: tick size resolution failed: %w", err)
	}

	halfWidth := state.config.gridHalfWidth

	if halfWidth <= 0 {
		derived := gridHalfWidthFromBook(bids, asks, tickSize)
		halfWidth = capGridHalfWidth(
			derived,
			state.config.bookDepthLevels,
			len(bids),
			len(asks),
		)
	}

	if halfWidth <= 0 {
		return fmt.Errorf("fluid: %s book-derived grid half width must be positive", state.symbol)
	}

	if err := state.configureGrid(tickSize, halfWidth); err != nil {
		return err
	}

	return nil
}

func NewFluidSymbol(symbol string) (*FluidSymbol, error) {
	symbolConfig, configErr := loadSymbolConfig()

	if configErr != nil {
		return nil, configErr
	}

	state := &FluidSymbol{
		symbol: symbol,
		flux:   newFluxAccumulator(),
		config: symbolConfig,
	}

	if symbolConfig.tickSizeFallback <= 0 || symbolConfig.gridHalfWidth <= 0 {
		return state, nil
	}

	grid, err := newFluidGrid(
		symbolConfig.tickSizeFallback,
		symbolConfig.gridHalfWidth,
		symbolConfig.integrationInterval,
		symbolConfig.idleThreshold,
		symbolConfig.maxIntegrationSteps,
	)

	if err != nil {
		return nil, err
	}

	state.grid = grid

	return state, nil
}

/*
ConfigureTick builds or rebuilds the price lattice from the exchange price increment.
After the book is live, tick size is fixed unless the grid has not been created yet.
*/
func (state *FluidSymbol) ConfigureTick(priceIncrement float64) error {
	return state.configureGrid(priceIncrement, state.config.gridHalfWidth)
}

/*
configureGrid creates or remaps the fluid grid for the current tick size so
retained state stays aligned with prices.
*/
func (state *FluidSymbol) configureGrid(
	priceIncrement float64,
	halfWidth int,
) error {
	if priceIncrement <= 0 {
		return fmt.Errorf("fluid: %s price increment must be positive", state.symbol)
	}

	if state.bookReady && state.grid != nil {
		return nil
	}

	if state.grid != nil && state.grid.tickSize == priceIncrement {
		return nil
	}

	if halfWidth <= 0 {
		return fmt.Errorf("fluid: %s book-derived grid half width must be positive", state.symbol)
	}

	integrationInterval := state.config.integrationInterval

	if integrationInterval <= 0 {
		return fmt.Errorf("fluid: signals.fluid.integration_interval must be positive")
	}

	grid, err := newFluidGrid(
		priceIncrement,
		halfWidth,
		integrationInterval,
		state.config.idleThreshold,
		state.config.maxIntegrationSteps,
	)

	if err != nil {
		return err
	}

	state.grid = grid

	return nil
}

/*
FeedTicker updates price and volume context from a ticker event so fluid
measurements use current market scale.
*/
func (state *FluidSymbol) FeedTicker(row kraken.TickerData, at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("fluid: ticker event time is zero")
	}

	state.lastEventAt = at

	state.changePct = row.ChangePct
	state.volume = row.Volume

	if row.Volume > 0 {
		barsPerDay := state.config.volumeBarsPerDay

		if barsPerDay <= 0 {
			return errnie.Error(fmt.Errorf("fluid: signals.volume_clock_bars_per_day must be positive"))
		}

		if err := state.flux.setTarget(row.Volume / barsPerDay); err != nil {
			return err
		}
	}

	lastPrice := row.Last.Float64()

	if lastPrice > 0 {
		state.last = lastPrice
		state.recordPriceMemory(lastPrice)
	}

	bidPrice := row.Bid.Float64()

	if bidPrice > 0 {
		state.bid = bidPrice
	}

	askPrice := row.Ask.Float64()

	if askPrice > 0 {
		state.ask = askPrice
	}

	return nil
}

/*
FeedBook validates and applies a book event so resting liquidity advances the
symbol's fluid state.
*/
func (state *FluidSymbol) FeedBook(update kraken.BookData, at time.Time) error {
	return state.feedBookLocked(update, at)
}

func applyBookDeltas(current []kraken.BookLevel, deltas []kraken.BookLevel, isAsk bool) []kraken.BookLevel {
	if len(deltas) == 0 {
		return current
	}

	levels := make(map[string]kraken.BookLevel)
	for _, level := range current {
		levels[level.Price.String()] = level
	}

	for _, delta := range deltas {
		if delta.Qty == 0 {
			delete(levels, delta.Price.String())
		} else {
			levels[delta.Price.String()] = delta
		}
	}

	result := make([]kraken.BookLevel, 0, len(levels))
	for _, level := range levels {
		result = append(result, level)
	}

	if isAsk {
		// Asks: ascending (lowest price first)
		slices.SortFunc(result, func(a, b kraken.BookLevel) int {
			return a.Price.Cmp(&b.Price)
		})
	} else {
		// Bids: descending (highest price first)
		slices.SortFunc(result, func(a, b kraken.BookLevel) int {
			return b.Price.Cmp(&a.Price)
		})
	}

	return result
}

/*
feedBookLocked applies one validated book update while the caller exclusively
owns symbol state.
*/
func (state *FluidSymbol) feedBookLocked(update kraken.BookData, at time.Time) error {
	if update.PriceIncrement.Float64() > 0 {
		state.instrumentTickSize = update.PriceIncrement.Float64()
	}

	if update.Type != "snapshot" && update.Type != "update" {
		return fmt.Errorf("fluid: book frame type must be snapshot or update")
	}

	if update.Type == "snapshot" {
		book := update
		state.book = &book
		state.bookReady = true
	}

	if !state.bookReady {
		return nil
	}

	if at.IsZero() {
		return fmt.Errorf("fluid: book event time is zero")
	}

	state.lastEventAt = at

	flux := 0.0

	if update.Type != "snapshot" {
		if len(update.Bids) > 0 {
			flux += state.trustedSideChangeFlux(state.book.Bids, update.Bids)
		}

		if len(update.Asks) > 0 {
			flux += state.trustedSideChangeFlux(state.book.Asks, update.Asks)
		}
	}

	bids := update.Bids
	asks := update.Asks

	if update.Type == "update" {
		bids = applyBookDeltas(state.book.Bids, update.Bids, false)
		asks = applyBookDeltas(state.book.Asks, update.Asks, true)
	}

	state.updateTouchLocked(bids, asks)

	if update.Type == "snapshot" {
		if err := state.configureTickFromBook(bids, asks); err != nil {
			return err
		}
	}

	mid := (state.bid + state.ask) / 2

	if mid <= 0 {
		return nil
	}

	if state.grid == nil {
		return nil
	}

	if ingestErr := state.grid.ingestBook(bids, asks, mid, at); ingestErr != nil {
		return ingestErr
	}

	state.book = &kraken.BookData{
		Symbol:    update.Symbol,
		Type:      update.Type,
		Timestamp: update.Timestamp,
		Bids:      bids,
		Asks:      asks,
	}

	if flushErr := state.flushBufferedTrades(); flushErr != nil {
		return flushErr
	}

	if flux <= 0 || !state.flux.hasTarget() {
		return nil
	}

	return state.flux.addBook(flux)
}

/*
updateTouchLocked records best bid and ask levels so spread-sensitive dynamics
use the current touch.
*/
func (state *FluidSymbol) updateTouchLocked(bids, asks []kraken.BookLevel) {
	if len(bids) == 0 || len(asks) == 0 {
		return
	}

	bid := bids[0].Price.Float64()
	ask := asks[0].Price.Float64()
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

/*
FeedTrade validates and applies a trade event so executed flow advances the
symbol's fluid state.
*/
func (state *FluidSymbol) FeedTrade(
	at time.Time,
	price, qty float64,
	side string,
) error {
	if !at.IsZero() {
		state.lastEventAt = at
	}

	if !state.bookReady || state.grid == nil || state.grid.lastMidPrice <= 0 {
		state.bufferTrade(at, price, qty, side)

		return nil
	}

	if state.grid.priceIndex(state.grid.lastMidPrice, price) < 0 {
		state.bufferTrade(at, price, qty, side)

		return nil
	}

	return state.applyTrade(at, price, qty, side)
}

/*
bufferTrade retains a trade arriving before usable book state so event
ordering is preserved explicitly.
*/
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

/*
applyTrade projects one validated trade into the configured grid so executions
affect density and velocity.
*/
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

/*
flushBufferedTrades applies retained pre-book trades after grid initialization
so authoritative events are not discarded.
*/
func (state *FluidSymbol) flushBufferedTrades() error {
	if len(state.bufferedTrades) == 0 || state.grid == nil || state.grid.lastMidPrice <= 0 {
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

/*
HasBook reports whether the symbol has usable book state before consumers
request a fluid reading.
*/
func (state *FluidSymbol) HasBook() bool {
	return state.bookReady
}

/*
Row returns the current wire representation so registry snapshots expose the
state used by measurements.
*/
func (state *FluidSymbol) Row() map[string]any {
	if state.lastEventAt.IsZero() {
		return nil
	}

	return state.wireRowLocked()
}

/*
Reading returns a coherent fluid reading only after the symbol has sufficient
initialized state.
*/
func (state *FluidSymbol) Reading() (fluidReading, bool) {
	if state.last <= 0 || state.lastEventAt.IsZero() {
		return fluidReading{}, false
	}

	if state.spreadBPS <= 0 {
		return fluidReading{}, false
	}

	if state.grid == nil || !state.grid.ready() {
		return fluidReading{}, false
	}

	spread := state.ask - state.bid

	if spread <= 0 {
		return fluidReading{}, false
	}

	divergence := state.grid.midVelocityDivergence()
	viscosity := state.fluidViscosity(spread)

	if math.IsNaN(viscosity) || math.IsInf(viscosity, 0) || viscosity < 0 {
		return fluidReading{}, false
	}

	// Zero touch-band replenishment this step forces an undefined
	// inertial/viscous ratio (division by zero) rather than a NaN, so it is
	// caught here explicitly and reported as the reflexive "no measurable
	// flow relative to damping yet" boundary: reynolds is left at its
	// zero-value and downstream classification falls to the score-based
	// (laminar/inertial) path instead of the ratio-based one.
	reynolds := 0.0

	if viscosity > 0 {
		reynolds = state.grid.reynoldsAgainst(spread, viscosity)
	}

	if math.IsNaN(reynolds) || math.IsInf(reynolds, 0) || reynolds < 0 {
		return fluidReading{}, false
	}

	if math.IsNaN(divergence) || math.IsInf(divergence, 0) {
		return fluidReading{}, false
	}

	state.dynamics.record(
		state.lastEventAt,
		reynolds,
		math.Abs(divergence),
		viscosity,
		math.Abs(state.grid.midVelocityCurvature()),
		state.grid.turbulenceIntensity(),
		state.grid.midAddRateAtTouch(),
		state.grid.midExecuteRateAtTouch(),
	)

	return fluidReading{
		symbol:            state.symbol,
		price:             state.last,
		spreadBPS:         state.spreadBPS,
		volume:            state.volume,
		changePct:         state.changePct,
		reynolds:          reynolds,
		divergence:        divergence,
		viscosity:         viscosity,
		velocityCurvature: math.Abs(state.grid.midVelocityCurvature()),
		turbulence:        state.grid.turbulenceIntensity(),
		sourceBalance:     state.grid.midSourceBalance(),
		memory:            priceMemoryFromSamples(state.memorySamples),
		midAddRate:        state.grid.midAddRateAtTouch(),
		midExecuteRate:    state.grid.midExecuteRateAtTouch(),
		dynamics:          state.dynamics,
		gridSteps:         state.grid.steps(),
	}, true
}

/*
wireRowLocked assembles current wire fields while the caller owns symbol
state.
*/
func (state *FluidSymbol) wireRowLocked() map[string]any {
	if state.last <= 0 || state.grid == nil || !state.grid.ready() {
		return nil
	}

	spread := state.ask - state.bid

	if spread <= 0 {
		return nil
	}

	divergence := state.grid.midVelocityDivergence()
	viscosity := state.fluidViscosity(spread)
	reynolds := state.grid.reynoldsAgainst(spread, viscosity)

	if math.IsNaN(reynolds) || math.IsInf(reynolds, 0) {
		return nil
	}

	return WireRow(map[string]any{
		"symbol":                state.symbol,
		"change_pct":            state.changePct,
		"vol":                   state.volume,
		"div_v2":                divergence,
		"velocity_curvature_v2": state.grid.midVelocityCurvature(),
		"turb":                  state.grid.turbulenceIntensity(),
		"visc":                  viscosity,
		"re":                    reynolds,
		"src_bal":               state.grid.midSourceBalance(),
	})
}

/*
fluidViscosity derives viscosity from grid dynamics and spread so the
published value is market-scaled.
*/
func (state *FluidSymbol) fluidViscosity(spread float64) float64 {
	spreadViscosity := state.spreadViscosity(spread)
	replenishment := state.grid.viscosity()

	if replenishment > 0 {
		return spreadViscosity * (1 + replenishment)
	}

	return spreadViscosity
}

/*
spreadViscosity derives the spread contribution to viscosity so price
separation is represented dimensionally.
*/
func (state *FluidSymbol) spreadViscosity(spread float64) float64 {
	mid := (state.bid + state.ask) / 2

	if mid <= 0 || spread <= 0 {
		return 0
	}

	return mid / spread
}

const fluidMemorySampleCap = 32

/*
recordPriceMemory updates retained price history so adaptive dynamics use
observed movement rather than a fixed window.
*/
func (state *FluidSymbol) recordPriceMemory(price float64) {
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return
	}

	state.memorySamples = append(state.memorySamples, price)

	if len(state.memorySamples) > fluidMemorySampleCap {
		state.memorySamples = state.memorySamples[len(state.memorySamples)-fluidMemorySampleCap:]
	}
}

func priceMemoryFromSamples(samples []float64) float64 {
	if len(samples) < 3 {
		return 0
	}

	minVal := samples[0]
	maxVal := samples[0]

	for _, sample := range samples {
		if sample < minVal {
			minVal = sample
		}

		if sample > maxVal {
			maxVal = sample
		}
	}

	span := maxVal - minVal

	if span <= 0 {
		return 0
	}

	normalized := make([]float64, len(samples))

	for index, sample := range samples {
		normalized[index] = (sample - minVal) / span
	}

	value, ready, err := adaptive.FractionalDifferenceValue(normalized)

	if err != nil || !ready {
		return 0
	}

	return math.Abs(value)
}
