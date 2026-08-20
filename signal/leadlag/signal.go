package leadlag

import (
	"context"

	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	thesis  *types.Thesis
	section *Section
	work    *transport.Consumer[*types.Symbol]
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:     ctx,
		cancel:  cancel,
		thesis:  thesis,
		section: NewSection(),
	}
	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceLeadLag).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceLeadLag) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceLeadLag }

func (signal *Signal) consume() {
	go func() {
		for symbol := range signal.thesis.Work(types.SourceLeadLag).Drain(signal.work, nil) {
			select {
			case <-signal.ctx.Done():
				signal.err = signal.ctx.Err()
				return
			default:
			}

			if symbol == nil {
				continue
			}

			for ticker := range symbol.MarketTickers(
				symbol.TickerConsumers[types.TickerConsumerLeadLag],
			) {
				anchor := signal.section.CausalAnchor()

				if anchor == "" {
					signal.section.ClearAnchor()
				} else {
					signal.section.SetAnchor(anchor)
				}

				signal.section.ObservePrice(
					symbol.Symbol,
					ticker.Last.Float64(),
					ticker.Timestamp,
				)
				symbol.AppendMeasurement(signal.measurement(symbol.Symbol, ticker.Timestamp))
			}
		}
	}()
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
