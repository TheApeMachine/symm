package correlation

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
	sample  *algorithm.CohortSample
	process *equation.Cohort
}

func NewSignal(ctx context.Context) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "correlation"),
		sample:  algorithm.NewCohortSample(),
		process: equation.NewCohort(),
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

		system.Q.Submit(func() {
			defer wg.Done()

			if data.Price.IsZero() {
				return
			}

			price, _ := data.Price.Float64()

			if price <= 0 {
				return
			}

			frame, ok, err := signal.sample.Measure(algorithm.CohortSampleInput{
				Symbol: data.Symbol,
				Price:  price,
				At:     data.Timestamp,
			})

			if err != nil || !ok {
				signal.Error(err)
				return
			}

			outcome, err := signal.process.Measure(frame)

			if err != nil {
				signal.Error(err)
				return
			}

			if !outcome.Eligible {
				return
			}

			mu.Lock()
			envelope.Correlation[data.Symbol] = &outcome
			mu.Unlock()
		})
	}

	wg.Wait()

	ui.UI.Write(envelope.Publish("correlation"))
	return envelope
}
