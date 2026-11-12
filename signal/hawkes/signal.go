package hawkes

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Signal is the Hawkes disruptor stage — outcomes land on envelope.Hawkes and
Map() feeds logic/resonance Features (spectral_radius) for Model story gates.
*/
type Signal struct {
	*runtime.System
	sample  *excitation.Sample
	process *excitation.Process
}

/*
NewSignal is tape-only — Hawkes builds cross-symbol or print statistics without
a book callback on the hot path.
*/
func NewSignal(ctx context.Context) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "hawkes"),
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

	for _, data := range envelope.Trade.Data {
		wg.Go(func() {
			input, ok, err := signal.sample.MeasureArrival(excitation.TradeInput{
				Symbol:    data.Symbol,
				Side:      data.Side.String(),
				Timestamp: data.Timestamp,
			})

			if err != nil {
				signal.Error(err)
				return
			}

			if !ok {
				return
			}

			outcome, ready, err := signal.process.Measure(input)

			if err != nil {
				signal.Error(err)
				return
			}

			if !ready {
				return
			}

			mu.Lock()
			envelope.Hawkes[data.Symbol] = &outcome
			mu.Unlock()
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Hawkes[focus]; ok {
		ui.Focus("hawkes", focus, reading)
	}

	return envelope
}
