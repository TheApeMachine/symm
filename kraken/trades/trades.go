package trades

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/client"
	"github.com/theapemachine/symm/kraken/market"
)

const (
	sideBuy       = "buy"
	sideSell      = "sell"
	tradeEventCap = 512
)

/*
Trades watches Kraken v2 trade executions and exposes per-symbol buy pressure.
*/
type Trades struct {
	mu      sync.RWMutex
	err     error
	symbols map[string]tradeState
	ready   int
}

type tradeState struct {
	pressure float64
	volume   float64
	ready    bool
	ticks    []market.TradeTick
}

type tradeBatch struct {
	buyVolume float64
	volume    float64
}

/*
New subscribes on the shared public websocket and registers the trade handler.
*/
func New(
	_ context.Context,
	publicClient *client.PublicClient,
	symbols []string,
) (*Trades, error) {
	switch {
	case len(symbols) == 0:
		return nil, fmt.Errorf("trade observer requires at least one symbol")
	case publicClient == nil:
		return nil, fmt.Errorf("public websocket client is nil")
	}

	if err := publicClient.SubscribeTo(market.SubscribeParams{}.Trades(symbols)); err != nil {
		return nil, fmt.Errorf("subscribe trade channel: %w", err)
	}

	trades := &Trades{
		symbols: make(map[string]tradeState, len(symbols)),
	}

	for _, symbol := range symbols {
		trades.symbols[symbol] = tradeState{}
	}

	publicClient.OnFrame(trades.handleFrame)

	return trades, nil
}

/*
Observe returns mean buy pressure across ready symbols in [-1, 1].
*/
func (trades *Trades) Observe(_ context.Context) (engine.Observation, error) {
	trades.mu.RLock()
	defer trades.mu.RUnlock()

	switch {
	case trades.err != nil:
		return engine.Observation{}, trades.err
	case trades.ready == 0:
		return engine.Observation{}, fmt.Errorf("trade observer not ready")
	}

	var sum float64

	for _, state := range trades.symbols {
		if state.ready {
			sum += state.pressure
		}
	}

	return engine.Observation{
		Confidence: sum / float64(trades.ready),
	}, nil
}

/*
BuyPressure returns executed buy pressure for one symbol in [-1, 1].
*/
func (trades *Trades) BuyPressure(symbol string) (float64, bool) {
	state, ok := trades.state(symbol)

	if !ok {
		return 0, false
	}

	return state.pressure, true
}

/*
BatchVolume returns executed volume from the latest trade batch for one symbol.
*/
func (trades *Trades) BatchVolume(symbol string) (float64, bool) {
	state, ok := trades.state(symbol)

	if !ok {
		return 0, false
	}

	return state.volume, true
}

/*
RecentTicks returns stored trade events for one symbol, optionally filtered by time.
*/
func (trades *Trades) RecentTicks(symbol string, since time.Time) ([]market.TradeTick, bool) {
	trades.mu.RLock()
	defer trades.mu.RUnlock()

	state, ok := trades.symbols[symbol]

	if !ok || !state.ready {
		return nil, false
	}

	if since.IsZero() {
		cp := append([]market.TradeTick(nil), state.ticks...)
		return cp, len(cp) > 0
	}

	filtered := make([]market.TradeTick, 0, len(state.ticks))

	for _, tick := range state.ticks {
		if !tick.Timestamp.Before(since) {
			filtered = append(filtered, tick)
		}
	}

	return filtered, len(filtered) > 0
}

func (trades *Trades) state(symbol string) (tradeState, bool) {
	trades.mu.RLock()
	defer trades.mu.RUnlock()

	state, ok := trades.symbols[symbol]

	if !ok || !state.ready {
		return tradeState{}, false
	}

	return state, true
}

func (trades *Trades) handleFrame(_ context.Context, payload []byte) error {
	ticks, err := market.ParseTrades(payload)

	switch {
	case errors.Is(err, market.ErrNotTrade):
		return nil
	case err != nil:
		return trades.fail(fmt.Errorf("parse trades frame: %w", err))
	case len(ticks) == 0:
		return nil
	default:
		return trades.applyTicks(ticks)
	}
}

func (trades *Trades) applyTicks(ticks []market.TradeTick) error {
	batches := make(map[string]tradeBatch)

	for _, tick := range ticks {
		batch := batches[tick.Symbol]
		batch.volume += tick.Volume

		switch tick.Side {
		case sideBuy:
			batch.buyVolume += tick.Volume
		case sideSell:
		default:
			return trades.fail(fmt.Errorf("unknown trade side %q", tick.Side))
		}

		batches[tick.Symbol] = batch
	}

	trades.mu.Lock()
	defer trades.mu.Unlock()

	for symbol, batch := range batches {
		if batch.volume <= 0 {
			trades.err = fmt.Errorf("trade batch for %s has zero volume", symbol)
			return trades.err
		}

		state := trades.symbols[symbol]

		if !state.ready {
			trades.ready++
		}

		state.ready = true
		state.volume = batch.volume
		state.pressure = (2*batch.buyVolume - batch.volume) / batch.volume

		for _, tick := range ticks {
			if tick.Symbol != symbol {
				continue
			}

			state.ticks = append(state.ticks, tick)
		}

		if len(state.ticks) > tradeEventCap {
			state.ticks = state.ticks[len(state.ticks)-tradeEventCap:]
		}

		trades.symbols[symbol] = state
	}

	return nil
}

func (trades *Trades) fail(err error) error {
	trades.mu.Lock()
	trades.err = err
	trades.mu.Unlock()

	return err
}
