package correlation

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
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
Step observes every print and heartbeat mark (cheap), then emits cohort frames
only for symbols that actually traded on this tick. Heartbeat used to force a
full peer matrix per mark and stalled the signal ring for hundreds of ms.
*/
func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	inputs := make([]algorithm.CohortSampleInput, 0, 64)
	traded := make([]string, 0, 8)

	for _, data := range envelope.Trade.Data {
		if data.Price.IsZero() {
			continue
		}

		price, _ := data.Price.Float64()

		if price <= 0 || data.Symbol == "" {
			continue
		}

		at := data.Timestamp

		if at.IsZero() {
			at = time.Now()
		}

		inputs = append(inputs, algorithm.CohortSampleInput{
			Symbol: data.Symbol,
			Price:  price,
			At:     at,
		})
		traded = append(traded, data.Symbol)
	}

	if signal.price != nil {
		batch := system.Cfg.Selection.HeartbeatBatch

		if batch <= 0 {
			batch = 48
		}

		start := int(signal.pulse.Load())
		symbols, next := signal.price.PulseSymbols(batch, start)

		for _, symbol := range symbols {
			mark := signal.price.Mark(symbol)
			at := signal.quoteTime(symbol)

			if mark == nil || mark.Sign() <= 0 || at.IsZero() {
				continue
			}

			inputs = append(inputs, algorithm.CohortSampleInput{
				Symbol: symbol,
				Price:  mark.InexactFloat64(),
				At:     at,
			})
		}

		signal.pulse.Store(uint64(next))
	}

	if err := signal.sample.ObserveMany(inputs); err != nil {
		signal.Error(err)
	}

	frames, err := signal.sample.EmitMany(traded)

	if err != nil {
		signal.Error(err)
	}

	for symbol, frame := range frames {
		outcome, err := signal.process.Measure(frame)

		if err != nil {
			signal.Error(err)
			continue
		}

		envelope.Correlation[symbol] = &outcome
	}

	focus := types.Focus()
	if reading, ok := envelope.Correlation[focus]; ok {
		ui.Focus("correlation", focus, reading)
	}

	return envelope
}

/*
quoteTime is the latest exchange touch time. Heartbeats without it are skipped
so a local clock cannot walk the cohort sample backwards.
*/
func (signal *Signal) quoteTime(symbol string) time.Time {
	var at time.Time

	if signal.price == nil || signal.price.Books == nil {
		return at
	}

	signal.price.Books.Book(symbol, func(book *market.Book) {
		if book == nil {
			return
		}

		if bid := book.BestBid(); bid != nil && !bid.Timestamp.IsZero() && bid.Timestamp.After(at) {
			at = bid.Timestamp
		}

		if ask := book.BestAsk(); ask != nil && !ask.Timestamp.IsZero() && ask.Timestamp.After(at) {
			at = ask.Timestamp
		}
	})

	return at
}
