package sentiment

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
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
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, data := range envelope.Trade.Data {
		wg.Add(1)

		system.Q.Submit(func() {
			defer wg.Done()

			if data.Price.IsZero() {
				return
			}

			price, _ := data.Price.Float64()
			if price <= 0 {
				return
			}

			input, ok, err := signal.sample.Measure(algorithm.ConvictionSampleInput{
				Symbol: data.Symbol,
				Price:  price,
				At:     data.Timestamp,
			})

			if err != nil || !ok {
				signal.Error(err)
				return
			}

			outcome, err := signal.process.Measure(input)
			if err != nil {
				signal.Error(err)
				return
			}

			mu.Lock()
			envelope.Sentiment[data.Symbol] = &outcome
			mu.Unlock()
		})
	}

	wg.Wait()

	ui.UI.Write(envelope.Publish("sentiment"))
	return envelope
}
