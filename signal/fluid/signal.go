package fluid

import (
	"context"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal is a fluid signal that observes market data and calculates measurements.
*/
type Signal struct {
	tickerIn chan []kraken.TickerData
	bookIn   chan []kraken.BookData
	tradeIn  chan []kraken.TradeData
	ack     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	registry *Registry
	ticker   *Ticker
	trade    *Trade
	book     *Book
	ui       chan []byte
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.ForPublish(measurements),
	}.Marshal():
	default:
	}
}

/*
NewSignal creates Fluid's calculators for the central immutable market cuts.
Transport ingestion and instrument enrichment remain owned by the production
market path, so the signal only receives its lifecycle context and UI channel.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	registry := NewSyncRegistry()

	signal := &Signal{
		tickerIn: make(chan []kraken.TickerData, 64),
		bookIn:   make(chan []kraken.BookData, 64),
		tradeIn:  make(chan []kraken.TradeData, 64),
		ack:     make(chan struct{}, 256),
		ctx:      ctx,
		cancel:   cancel,
		registry: registry,
		ui:       ui,
		ticker:   NewTicker(registry),
		trade:    NewTrade(registry),
		book:     NewBook(registry),
	}

	return signal
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}

/*
Tickers returns the ticker ingress channel.
*/
func (signal *Signal) Tickers() chan []kraken.TickerData {
	return signal.tickerIn
}

/*
Books returns the book ingress channel.
*/
func (signal *Signal) Books() chan []kraken.BookData {
	return signal.bookIn
}

/*
Trades returns the trade ingress channel.
*/
func (signal *Signal) Trades() chan []kraken.TradeData {
	return signal.tradeIn
}


/*
Ack signals that one ingress frame finished Calculate so Crypto can barrier
before draining outs.
*/
func (signal *Signal) Ack() <-chan struct{} {
	return signal.ack
}

/*
Measure consumes ingress channels and sends measurements on out.
*/
func (signal *Signal) Measure() chan []*types.Measurement {
	out := make(chan []*types.Measurement, 64)

	go func() {
		defer close(out)

		for {
			select {
			case <-signal.ctx.Done():
				return
			case rows := <-signal.tickerIn:
				measured, err := signal.Calculate(rows, nil, nil)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			case rows := <-signal.bookIn:
				measured, err := signal.Calculate(nil, nil, rows)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			case rows := <-signal.tradeIn:
				measured, err := signal.Calculate(nil, rows, nil)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			}
		}
	}()

	return out
}
