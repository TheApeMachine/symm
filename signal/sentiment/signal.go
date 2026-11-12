package sentiment

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Signal is the Sentiment disruptor stage — outcomes land on envelope.Sentiment and
Map() feeds logic/resonance Features (slump_score) for Model story gates.
*/
type Signal struct {
	*runtime.System
	sample  *algorithm.ConvictionSample
	process *equation.Conviction
}

/*
NewSignal is tape-only — Sentiment builds cross-symbol or print statistics without
a book callback on the hot path.
*/
func NewSignal(ctx context.Context) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "sentiment"),
		sample:  algorithm.NewConvictionSample(),
		process: equation.NewConviction(),
	}
}

/*
Step fans one trade frame in parallel — metrics aggregate later, but each symbol
needs fresh readings here or featureScorer never marks that Feature as seen.

*/
func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, data := range envelope.Trade.Data {
		wg.Go(func() {
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

			if err != nil {
				signal.Error(err)
				return
			}

			if !ok {
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

	focus := types.Focus()
	if reading, ok := envelope.Sentiment[focus]; ok {
		ui.Focus("sentiment", focus, reading)
	}

	return envelope
}
