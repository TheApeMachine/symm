package sentiment

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/signal/frame"
	"github.com/theapemachine/symm/ui"
)

type Signal struct {
	*runtime.System
	sample  *algorithm.ConvictionSample
	process *equation.Conviction
}

func NewSignal(ctx context.Context) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "sentiment"),
		sample:  algorithm.NewConvictionSample(),
		process: equation.NewConviction(),
	}
}

func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var mu sync.Mutex

	frame.NewSymbols(envelope.Trade.Data).Each(func(trades []*market.TradeData) {
		for _, data := range trades {
			if data.Price.IsZero() {
				continue
			}

			price, _ := data.Price.Float64()
			if price <= 0 {
				continue
			}

			input, ok, err := signal.sample.Measure(algorithm.ConvictionSampleInput{
				Symbol: data.Symbol,
				Price:  price,
				At:     data.Timestamp,
			})

			if err != nil {
				signal.Error(err)
				continue
			}

			if !ok {
				continue
			}

			outcome, err := signal.process.Measure(input)
			if err != nil {
				signal.Error(err)
				continue
			}

			mu.Lock()
			envelope.Sentiment[data.Symbol] = &outcome
			mu.Unlock()
		}
	})

	ui.UI.Write(envelope.Publish("sentiment"))
	return envelope
}
