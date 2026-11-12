package cvd

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Signal is the CVD disruptor stage — outcomes land on envelope.CVD and
Map() feeds logic/resonance Features (starvation) for Model story gates.
*/
type Signal struct {
	*runtime.System
	books   *broker.Book
	sample  *algorithm.TradeFlowSample
	process *equation.Flow
}

/*
NewSignal binds L3 touch state on each trade tick — CVD labels are book-native,
not printable from public tape alone.
*/
func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "cvd"),
		books:   books,
		sample:  algorithm.NewTradeFlowSample(),
		process: equation.NewFlow(),
	}
}

/*
Step ties each trade to L3 mid — starvation is flow vs touch, not raw CVD on
prints alone, so the metric matches exit-side pressure in the forward story.
*/
func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, data := range envelope.Trade.Data {
		wg.Go(func() {
			if data.Price.IsZero() || data.Qty.IsZero() {
				return
			}

			price := data.Price.InexactFloat64()
			qty := data.Qty.InexactFloat64()

			if price <= 0 || qty <= 0 {
				return
			}

			var mid float64

			signal.books.Book(data.Symbol, func(book *market.Book) {
				midDec := book.Midpoint()
				if midDec == nil {
					return
				}

				mid = midDec.InexactFloat64()
			})

			if mid <= 0 {
				return
			}

			input, ok, err := signal.sample.Measure(algorithm.TradeFlowInput{
				Symbol:        data.Symbol,
				Price:         price,
				ResponsePrice: mid,
				Quantity:      qty,
				Side:          data.Side.String(),
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

			mu.Lock()
			envelope.CVD[data.Symbol] = &outcome
			mu.Unlock()
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.CVD[focus]; ok {
		ui.Focus("cvd", focus, reading)
	}

	return envelope
}
