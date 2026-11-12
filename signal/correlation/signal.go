package correlation

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
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
	price   *broker.Price
	pulse   atomic.Uint64
}

/*
NewSignal builds cross-symbol cohort stats from prints and ticker heartbeats.
*/
func NewSignal(ctx context.Context, price *broker.Price) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "correlation"),
		sample:  algorithm.NewCohortSample(),
		process: equation.NewCohort(),
		price:   price,
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

	publish := func(symbol string, at time.Time, px float64) {
		if symbol == "" || px <= 0 {
			return
		}

		frameInput, ok, err := signal.sample.Measure(algorithm.CohortSampleInput{
			Symbol: symbol,
			Price:  px,
			At:     at,
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

		mu.Lock()
		envelope.Correlation[symbol] = &outcome
		mu.Unlock()
	}

	for _, data := range envelope.Trade.Data {
		wg.Go(func() {
			if data.Price.IsZero() {
				return
			}

			price, _ := data.Price.Float64()

			if price <= 0 {
				return
			}

			publish(data.Symbol, data.Timestamp, price)
		})
	}

	if signal.price != nil {
		batch := system.Cfg.Selection.HeartbeatBatch

		if batch <= 0 {
			batch = 48
		}

		start := int(signal.pulse.Load())
		symbols, next := signal.price.PulseSymbols(batch, start)
		now := time.Now()

		for _, symbol := range symbols {
			mark := signal.price.CloseMark(symbol)

			if mark == nil || mark.Sign() <= 0 {
				continue
			}

			symbol, px := symbol, mark.InexactFloat64()

			wg.Go(func() {
				publish(symbol, now, px)
			})
		}

		signal.pulse.Store(uint64(next))
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Correlation[focus]; ok {
		ui.Focus("correlation", focus, reading)
	}

	return envelope
}
