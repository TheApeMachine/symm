package leadlag

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Signal is the LeadLag disruptor stage — outcomes land on envelope.LeadLag and
Map() feeds logic/resonance Features (decoupled_score, sync_score) for Model story gates.
*/
type Signal struct {
	*runtime.System
	sample  *algorithm.LagSample
	process *algorithm.Lag
}

/*
NewSignal is tape-only — LeadLag builds cross-symbol or print statistics without
a book callback on the hot path.
*/
func NewSignal(ctx context.Context) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "leadlag"),
		sample:  algorithm.NewLagSample(),
		process: algorithm.NewLag(),
	}
}

/*
Step tracks cross-symbol lag on each print — contagion oracle needs decoupled and
sync scores even when the process marks a row ineligible this tick.
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

			input, ok, err := signal.sample.Measure(algorithm.LagSampleInput{
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

			// Publish ineligible/zero readings too — resonance holds until every
			// Map() label has been seen (decoupled_score / sync_score).
			mu.Lock()
			envelope.LeadLag[data.Symbol] = &outcome
			mu.Unlock()
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.LeadLag[focus]; ok {
		ui.Focus("leadlag", focus, reading)
	}

	return envelope
}
