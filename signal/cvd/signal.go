package cvd

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type Signal struct {
	*runtime.System
	books   *broker.Book
	sample  *algorithm.TradeFlowSample
	process *equation.Flow
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "cvd"),
		books:   books,
		sample:  algorithm.NewTradeFlowSample(),
		process: equation.NewFlow(),
	}
}

/*
Step processes every trade entry of the tick's frame; per-symbol flow state is
keyed by the entry's symbol and each ready outcome lands on the envelope.
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
		}

		if system.Q.Submit(task) != nil {
			task()
		}
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.CVD[focus]; ok {
		ui.Focus("cvd", focus, reading)
	}

	return envelope
}
