package correlation

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

			frameInput, ok, err := signal.sample.Measure(algorithm.CohortSampleInput{
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

			outcome, err := signal.process.Measure(frameInput)

			if err != nil {
				signal.Error(err)
				continue
			}

			if !outcome.Eligible {
				continue
			}

			mu.Lock()
			envelope.Correlation[data.Symbol] = &outcome
			mu.Unlock()
		}
	})

	ui.UI.Write(envelope.Publish("correlation"))
	return envelope
}
