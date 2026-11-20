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

	for _, batch := range envelope.Trade.Batches() {
		wg.Go(func() {
			for _, data := range batch {
				if data.Price.IsZero() || data.Qty.IsZero() {
					continue
				}

				price := data.Price.InexactFloat64()
				qty := data.Qty.InexactFloat64()

				if price <= 0 || qty <= 0 {
					continue
				}

				var response float64

				signal.books.Book(data.Symbol, func(book *market.Book) {
					response = microPriceFromBook(book)
				})

				if response <= 0 {
					continue
				}

				input, ok, err := signal.sample.Measure(algorithm.TradeFlowInput{
					Symbol:        data.Symbol,
					Price:         price,
					ResponsePrice: response,
					Quantity:      qty,
					Side:          data.Side.String(),
				})

				if err != nil {
					signal.Error(err)
					continue
				}

				if !ok {
					continue
				}

				outcome, err := signal.process.Measure(input)
				if err != nil {
					signal.Error(err)
					continue
				}

				mu.Lock()
				envelope.CVD[data.Symbol] = &outcome
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.CVD[focus]; ok {
		ui.Focus("cvd", focus, reading)
	}

	return envelope
}

/*
microPriceFromBook is Stoikov micro-price from touch queues; falls back to mid.
*/
func microPriceFromBook(book *market.Book) float64 {
	if book == nil || !book.Tradable() {
		return 0
	}

	bid := book.BestBid()
	ask := book.BestAsk()
	if bid == nil || ask == nil ||
		bid.Price == nil || ask.Price == nil ||
		bid.Quantity == nil || ask.Quantity == nil {
		if mid := book.Midpoint(); mid != nil {
			return mid.InexactFloat64()
		}
		return 0
	}

	bidPx := bid.Price.InexactFloat64()
	askPx := ask.Price.InexactFloat64()
	bidQty := bid.Quantity.InexactFloat64()
	askQty := ask.Quantity.InexactFloat64()
	if bidPx <= 0 || askPx <= bidPx || bidQty <= 0 || askQty <= 0 {
		if mid := book.Midpoint(); mid != nil {
			return mid.InexactFloat64()
		}
		return 0
	}

	return (askQty*bidPx + bidQty*askPx) / (bidQty + askQty)
}
