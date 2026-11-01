package hawkes

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/signal/frame"
	"github.com/theapemachine/symm/ui"
)

type Signal struct {
	*runtime.System
	sample  *excitation.Sample
	process *excitation.Process
}

func NewSignal(ctx context.Context) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "hawkes"),
		sample:  excitation.NewSample(),
		process: excitation.NewProcess(),
	}
}

/*
Step processes every trade entry of the tick's frame; per-symbol state is keyed
by the entry's symbol and each entry writes its own (reserved) layer.
*/
func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var mu sync.Mutex

	frame.NewSymbols(envelope.Trade.Data).Each(func(trades []*market.TradeData) {
		for _, data := range trades {
			input, ok, err := signal.sample.MeasureArrival(excitation.TradeInput{
				Symbol:    data.Symbol,
				Side:      data.Side.String(),
				Timestamp: data.Timestamp,
			})

			if err != nil {
				signal.Error(err)
				continue
			}

			if !ok {
				continue
			}

			outcome, ready, err := signal.process.Measure(input)

			if err != nil {
				signal.Error(err)
				continue
			}

			if !ready {
				continue
			}

			mu.Lock()
			envelope.Hawkes[data.Symbol] = &outcome
			mu.Unlock()
		}
	})

	ui.UI.Write(envelope.Publish("hawkes"))
	return envelope
}
