package manifold

import (
	"fmt"
	"math"
	"time"

	mkernel "github.com/theapemachine/nomagique/physics/manifold"
)

func (field *Field) RegisterSymbols(symbols []string) {
	field.universe.registerSymbols(symbols)
}

/*
SetInstrumentTick pins a symbol's price grid to the exchange-published tick_size,
overriding the book-gap inference. Ignored for non-positive ticks so a missing
value never clobbers a working derived tick.
*/
func (field *Field) SetInstrumentTick(symbol string, tickSize float64) {
	if field == nil || tickSize <= 0 {
		return
	}

	state := field.universe.loadSymbol(symbol)

	if state == nil {
		return
	}

	state.tickSize = tickSize
	state.tickPinned = true
}

func (field *Field) FeedTicker(row TickerUpdate, at time.Time) error {
	state := field.universe.loadSymbol(row.Symbol)

	if state == nil {
		return fmt.Errorf("manifold: symbol %q unavailable", row.Symbol)
	}

	if !at.IsZero() {
		state.lastEventAt = at
	}

	price := row.Last

	if price <= 0 {
		price = (row.Ask + row.Bid) / 2
	}

	if price <= 0 {
		return nil
	}

	field.recordPrice(state, price, at)

	return field.maybeStep(at)
}

func (field *Field) FeedBook(update BookUpdate, at time.Time) error {
	identity, err := SpotIdentityFromPair(update.Symbol)

	if err != nil {
		return fmt.Errorf("manifold: spot identity for %q: %w", update.Symbol, err)
	}

	return field.feedBookIdentity(identity, update, at)
}

func (field *Field) FeedFuturesBook(update BookUpdate, at time.Time) error {
	identity, err := FuturesIdentityFromProduct(update.Symbol)

	if err != nil {
		return fmt.Errorf("manifold: futures identity for %q: %w", update.Symbol, err)
	}

	return field.feedBookIdentity(identity, update, at)
}

func (field *Field) feedBookIdentity(
	identity InstrumentIdentity,
	update BookUpdate,
	at time.Time,
) error {
	state := field.universe.loadIdentity(identity)

	if state == nil {
		return fmt.Errorf("manifold: instrument %q unavailable", identity.Symbol)
	}

	if update.Type == "snapshot" {
		state.bookReady = true
		state.book = update
	}

	if !state.bookReady {
		return nil
	}

	if !at.IsZero() {
		state.lastEventAt = at
	}

	bids := update.Bids
	asks := update.Asks

	if len(bids) == 0 {
		bids = state.book.Bids
	}

	if len(asks) == 0 {
		asks = state.book.Asks
	}

	if len(bids) == 0 || len(asks) == 0 {
		return nil
	}

	midPrice := (bids[0].Price + asks[0].Price) / 2

	if midPrice <= 0 {
		return fmt.Errorf("manifold: mid price must be positive for %q", update.Symbol)
	}

	state.midPrice = midPrice

	// When the exchange tick_size is known it is ground truth; only infer the
	// tick from book gaps for pairs not yet catalogued by the instrument feed.
	if !state.tickPinned && (update.Type == "snapshot" || state.tickSize <= 0) {
		if err := state.configureTickFromBook(bids, asks, field.universe.tickSizeFallback()); err != nil {
			return err
		}
	}

	if state.lane == InstrumentLaneSpot {
		field.recordPrice(state, midPrice, at)
	}

	return field.maybeStep(at)
}

func (field *Field) FeedTrade(trade *TradeUpdate, at time.Time) error {
	state := field.universe.loadSymbol(trade.Symbol)

	if state == nil {
		return fmt.Errorf("manifold: symbol %q unavailable", trade.Symbol)
	}

	if !at.IsZero() {
		state.lastEventAt = at
	}

	field.recordPrice(state, trade.Price, at)
	state.recordTradeQty(trade.Qty, field.measurementsCapacity)

	if state.midPrice <= 0 {
		state.midPrice = trade.Price
	}

	if state.tickSize <= 0 {
		return fmt.Errorf("manifold: tick size must be positive for %q", trade.Symbol)
	}

	field.universe.recomputeRanksIfDirty()

	offsetTicks := (trade.Price - state.midPrice) / state.tickSize
	coords := field.universe.coords(state, offsetTicks)

	rho, rhoErr := field.liquidityRho(state, trade.Qty, 1)

	if rhoErr != nil {
		return rhoErr
	}

	if trade.Qty >= state.whaleQtyThreshold() {
		field.pendingWhales = append(field.pendingWhales, whaleCarrier{
			symbol: trade.Symbol,
			oscillator: field.whaleOscillatorFromTrade(
				state,
				trade,
				coords,
				rho,
			),
		})

		return field.maybeStep(at)
	}

	momentum := rho * tradeSideSign(trade.Side)

	field.pendingDeposits = append(field.pendingDeposits, cellDeposit{
		cellX: coords.cellX,
		cellY: coords.cellY,
		cellZ: coords.cellZ,
		rho:   rho,
		momX:  momentum,
		eInt:  rho * field.config.CV,
	})

	return field.maybeStep(at)
}

func (field *Field) Reading(symbol string) (mkernel.Reading, float64, time.Time, bool) {
	raw, ok := field.readings.Load(symbol)

	if !ok {
		return mkernel.Reading{}, 0, time.Time{}, false
	}

	row, rowOk := raw.(symbolReading)

	if !rowOk {
		return mkernel.Reading{}, 0, time.Time{}, false
	}

	return row.reading, row.price, row.at, true
}

func (field *Field) recordPrice(state *UniverseState, price float64, at time.Time) {
	if price <= 0 || at.IsZero() {
		return
	}

	if state.lastPrice <= 0 {
		state.lastPrice = price
		return
	}

	if price == state.lastPrice {
		return
	}

	logReturn := math.Log(price / state.lastPrice)
	state.lastPrice = price
	state.AppendReturn(logReturn, field.measurementsCapacity)

	field.universe.markRanksDirty()
}
