package fluid

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/types"
)

/*
Signal is a fluid signal that observes market data and calculates measurements.
*/
type Signal struct {
	ctx      context.Context
	cancel   context.CancelFunc
	registry *Registry
	ticker   *Ticker
	trade    *Trade
	book     *Book
	ui       chan []byte
}

/*
Interest requires ticker, trade, and book streams; the mechanical metrics merge
all three inputs into one causal event timeline per symbol.
*/
func (signal *Signal) Interest() types.StreamInterest {
	return types.StreamAll
}

/*
Measure returns typed measurements for the cut, or an error when the cut
cannot be measured honestly.
*/
func (signal *Signal) Measure(thesis *types.Thesis) ([]*types.Measurement, error) {
	return signal.Calculate(thesis.Market())
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
