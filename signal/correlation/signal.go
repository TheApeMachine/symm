package correlation

import (
	"context"
	"sync"
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
heartbeatStale is how old a touch/print clock may be before a quiet-symbol
heartbeat is skipped. Frozen resting-order timestamps must not keep feeding the
cohort with stale bid marks.
*/
const heartbeatStale = 30 * time.Second

/*
Signal is the Correlation disruptor stage — outcomes land on envelope.Correlation and
Map() feeds logic/resonance Features (herd_score, peak_score) for Model story gates.
*/
type Signal struct {
	*runtime.System
	sample   *algorithm.CohortSample
	process  *equation.Cohort
	price    *broker.Price
	pulse    atomic.Uint64
	lastMark sync.Map // symbol → float64 last heartbeat mark
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
		now := time.Now()

		for _, symbol := range symbols {
			mark := signal.price.Mark(symbol)
			at := signal.quoteTime(symbol, now)

			if mark == nil || mark.Sign() <= 0 || at.IsZero() {
				continue
			}

			px := mark.InexactFloat64()

			if raw, ok := signal.lastMark.Load(symbol); ok {
				if prev, ok := raw.(float64); ok && prev == px {
					continue
				}
			}

			signal.lastMark.Store(symbol, px)
			inputs = append(inputs, algorithm.CohortSampleInput{
				Symbol: symbol,
				Price:  px,
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
quoteTime is the newest of last print and live touch time, and only when that
clock is fresh. Resting BestBid/BestAsk timestamps freeze when the touch order
does not change — those must not keep walking the cohort with stale bids.
*/
func (signal *Signal) quoteTime(symbol string, now time.Time) time.Time {
	var at time.Time

	if signal.price == nil {
		return at
	}

	if print := signal.price.LastPrintAt(symbol); !print.IsZero() {
		at = print
	}

	if signal.price.Books != nil {
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
	}

	if at.IsZero() || now.Sub(at) > heartbeatStale {
		return time.Time{}
	}

	return at
}
