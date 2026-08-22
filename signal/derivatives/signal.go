package derivatives

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Signal converts real-time Kraken Futures streams (tickers, executions, and
order books) into multi-dimensional derivatives measurements. It tracks
open interest dynamics, spot-index-perp basis geometry, CVD aggressor flows,
liquidation bursts, and dynamic lead/lag correlation.
*/
type Signal struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	thesis   *types.Thesis
	states   map[string]*SymbolState
	statesMu sync.RWMutex
	work     *transport.Consumer[*types.Symbol]
	pool     *types.SymbolPool
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		states: make(map[string]*SymbolState),
		pool:   types.NewSymbolPool(types.ShardWorkers()),
	}

	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceDerivatives).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceDerivatives) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceDerivatives }

func (signal *Signal) getState(symbolName string) *SymbolState {
	signal.statesMu.RLock()
	state, exists := signal.states[symbolName]
	signal.statesMu.RUnlock()

	if exists {
		return state
	}

	signal.statesMu.Lock()
	state, exists = signal.states[symbolName]

	if !exists {
		state = NewSymbolState()
		signal.states[symbolName] = state
	}

	signal.statesMu.Unlock()
	return state
}

func (signal *Signal) consume() {
	go func() {
		defer func() {
			if err := signal.pool.Error(); err != nil {
				signal.err = err
			}

			signal.thesis.Fail(signal.err)
		}()

		for symbol := range signal.thesis.Work(types.SourceDerivatives).Drain(signal.work, nil) {
			select {
			case <-signal.ctx.Done():
				signal.pool.CaptureError(signal.ctx.Err())
				return
			default:
			}

			if symbol == nil {
				continue
			}

			symbolName := symbol.Symbol

			signal.pool.Submit(symbolName, func() {
				if err := signal.consumeSymbol(symbol); err != nil {
					signal.pool.CaptureError(errnie.Error(errnie.Err(
						errnie.Validation,
						"derivatives: condition "+symbolName,
						err,
					)))
				}
			})
		}
	}()
}

func (signal *Signal) consumeSymbol(symbol *types.Symbol) error {
	state := signal.getState(symbol.Symbol)
	updated := false
	latestTime := time.Time{}

	for ticker := range symbol.MarketFuturesTickers(
		symbol.FuturesTickerConsumers[types.FuturesTickerConsumerDerivatives],
	) {
		signal.processTicker(state, ticker)
		updated = true
		latestTime = ticker.Timestamp
	}

	for trade := range symbol.MarketFuturesTrades(
		symbol.FuturesTradeConsumers[types.FuturesTradeConsumerDerivatives],
	) {
		signal.processTrade(state, trade)
		updated = true

		if trade.Timestamp.After(latestTime) {
			latestTime = trade.Timestamp
		}
	}

	for book := range symbol.MarketFuturesBooks(
		symbol.FuturesBookConsumers[types.FuturesBookConsumerDerivatives],
	) {
		signal.processBook(state, book)
		updated = true

		if book.Timestamp.After(latestTime) {
			latestTime = book.Timestamp
		}
	}

	if !updated {
		return nil
	}

	measurement := BuildMeasurement(signal.Name(), symbol.Symbol, state, latestTime)
	symbol.AppendMeasurement(measurement)
	return nil
}

func (signal *Signal) processTicker(state *SymbolState, ticker kraken.FuturesTickerData) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if ticker.OpenInterest > 0 {
		state.PrevOpenInterest = state.LastOpenInterest
		state.LastOpenInterest = ticker.OpenInterest

		if state.PrevOpenInterest > 0 {
			prevOIDelta := state.OIDelta
			state.OIDelta = state.LastOpenInterest - state.PrevOpenInterest
			state.OIVelocity = state.OIDelta / state.PrevOpenInterest
			state.OIAcceleration = state.OIDelta - prevOIDelta
		}
	}

	if ticker.Last != nil && ticker.Last.Float64() > 0 {
		state.LastPerpPrice = ticker.Last.Float64()
	}

	if ticker.IndexPrice != nil && ticker.IndexPrice.Float64() > 0 {
		state.LastIndexPrice = ticker.IndexPrice.Float64()
	}

	if ticker.FundingRate != nil {
		state.LastFundingRate = ticker.FundingRate.Float64()
	}

	if state.LastSpotPrice > 0 && state.LastPerpPrice > 0 {
		state.PrevBasis = state.Basis
		state.Basis = (state.LastPerpPrice - state.LastSpotPrice) / state.LastSpotPrice
		state.BasisVelocity = state.Basis - state.PrevBasis

		if state.LastIndexPrice > 0 {
			state.IndexBasis = (state.LastIndexPrice - state.LastSpotPrice) / state.LastSpotPrice
			state.TripartiteDiv = (state.Basis - state.IndexBasis)
		}
	}

	state.LastUpdate = ticker.Timestamp
}

func (signal *Signal) processTrade(state *SymbolState, trade kraken.FuturesTradeData) {
	state.mu.Lock()
	defer state.mu.Unlock()

	price := trade.Price.Float64()
	notional := price * trade.Qty

	if trade.Side == "buy" {
		state.FuturesBuyVolume += notional
		state.FuturesCVD += notional
	}

	if trade.Side == "sell" {
		state.FuturesSellVolume += notional
		state.FuturesCVD -= notional
	}

	if trade.Type == "liquidation" {
		if trade.Side == "buy" {
			state.LiqBuyVolume += notional
		}

		if trade.Side == "sell" {
			state.LiqSellVolume += notional
		}
	}

	if price > 0 {
		state.LastPerpPrice = price
	}

	state.LastUpdate = trade.Timestamp
}

func (signal *Signal) processBook(state *SymbolState, book kraken.FuturesBookData) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		bidPrice := book.Bids[0].Price.Float64()
		askPrice := book.Asks[0].Price.Float64()
		mid := (bidPrice + askPrice) / 2.0

		if mid > 0 {
			state.LastPerpPrice = mid
		}
	}

	state.LastUpdate = book.Timestamp
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
