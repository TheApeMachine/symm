package hawkes

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Signal is the Hawkes disruptor stage — outcomes land on envelope.Hawkes and
Map() feeds logic/resonance Features (spectral_radius) for Model story gates.
*/
type Signal struct {
	*runtime.System
	price   *broker.Price
	sample  *excitation.Sample
	process *excitation.Process
}

/*
NewSignal scores tape arrivals; optional price caches spectral radius for
profit-only tranche pacing (never loss exits).
*/
func NewSignal(ctx context.Context, price *broker.Price) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "hawkes"),
		price:   price,
		sample:  excitation.NewSample(),
		process: excitation.NewProcess(),
	}
}

/*
Step feeds hawkes arrivals per print — spectral_radius is the hold-stability
feature; skipping quiet ticks would freeze the excitation state incorrectly.
*/
func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, batch := range envelope.Trade.Batches() {
		wg.Go(func() {
			for _, data := range batch {
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

				if signal.price != nil {
					signal.price.NoteExcitation(data.Symbol, outcome.Fit.SpectralRadius)
				}

				mu.Lock()
				envelope.Hawkes[data.Symbol] = &outcome
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Hawkes[focus]; ok {
		ui.Focus("hawkes", focus, reading)
	}

	return envelope
}
