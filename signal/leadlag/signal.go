package leadlag

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type Signal struct {
	*runtime.System
	sample  *algorithm.LagSample
	process *algorithm.Lag
}

func NewSignal(ctx context.Context) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "leadlag"),
		sample:  algorithm.NewLagSample(),
		process: algorithm.NewLag(),
	}
}

/*
Step processes every trade entry of the tick's frame; per-symbol state is keyed
by the entry's symbol and each ready outcome lands on the envelope.
*/
func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, data := range envelope.Trade.Data {
		wg.Add(1)

		task := func() {
			defer wg.Done()

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

			if !outcome.Eligible {
				return
			}

			mu.Lock()
			envelope.LeadLag[data.Symbol] = &outcome
			mu.Unlock()
		}

		if system.Q.Submit(task) != nil {
			task()
		}
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.LeadLag[focus]; ok {
		ui.Focus("leadlag", focus, reading)
	}

	return envelope
}
