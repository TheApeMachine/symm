package correlation

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
Signal is the Correlation disruptor stage — outcomes land on envelope.Correlation and
Map() feeds logic/resonance Features (herd_score, peak_score) for Model story gates.
*/
type Signal struct {
	*runtime.System
	sample  *algorithm.CohortSample
	process *equation.Cohort
}

/*
NewSignal is tape-only — Correlation builds cross-symbol or print statistics without
a book callback on the hot path.
*/
func NewSignal(ctx context.Context) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "correlation"),
		sample:  algorithm.NewCohortSample(),
		process: equation.NewCohort(),
	}
}

/*
Step fans cohort sampling per print — herd/peak need continuous cohort updates, and
ineligible rows still publish so resonance does not stall on missing Map() labels.
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

			frameInput, ok, err := signal.sample.Measure(algorithm.CohortSampleInput{
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

			outcome, err := signal.process.Measure(frameInput)

			if err != nil {
				signal.Error(err)
				return
			}

			// Publish ineligible/zero readings too — resonance holds until every
			// Map() label has been seen (herd_score / peak_score).
			mu.Lock()
			envelope.Correlation[data.Symbol] = &outcome
			mu.Unlock()
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Correlation[focus]; ok {
		ui.Focus("correlation", focus, reading)
	}

	return envelope
}
