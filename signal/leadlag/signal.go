package leadlag

import (
	"context"

	"github.com/theapemachine/symm/types"
)

type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	thesis  *types.Thesis
	section *Section
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:     ctx,
		cancel:  cancel,
		thesis:  thesis,
		section: NewSection(),
	}

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceLeadLag) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceLeadLag }

func (signal *Signal) Run() error {
	for signal.err == nil {
		symbol, available := signal.thesis.Work(types.SourceLeadLag).WaitPop(
			signal.ctx,
			string(types.SourceLeadLag),
		)

		if !available {
			return signal.ctx.Err()
		}

		if symbol == nil {
			continue
		}

		for ticker := range symbol.MarketTickers(types.SourceLeadLag) {
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

	return signal.err
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
