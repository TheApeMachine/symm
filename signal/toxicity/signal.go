package toxicity

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/algorithm/book/quality"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Signal is the Toxicity disruptor stage — outcomes land on envelope.Toxicity and
Map() feeds logic/resonance Features (vacuum_score) for Model story gates.
*/
type Signal struct {
	*runtime.System
	books   *broker.Book
	sample  *quality.Sample
	process *equation.BookQuality
}

/*
NewSignal binds L3 touch state on each trade tick — Toxicity labels are book-native,
not printable from public tape alone.
*/
func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "toxicity"),
		books:   books,
		sample:  quality.NewSample(),
		process: equation.NewBookQuality(),
	}
}

/*
Step pairs book shape with trade impact — vacuum_score is capacity veto input;
neutral readings still publish so featureScorer can finish the toxicity labels.
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

				var (
					bids      []flow.BookLevel
					asks      []flow.BookLevel
					bidOrders []quality.OrderEvent
					askOrders []quality.OrderEvent
					haveBook  bool
				)

				signal.books.Book(data.Symbol, func(book *market.Book) {
					if book == nil {
						return
					}

					bids, bidOrders = resting(book.Bids)
					asks, askOrders = resting(book.Asks)
					haveBook = true
				})

				if !haveBook {
					continue
				}

				if _, _, _, err := signal.sample.MeasureTrade(flow.TradeInput{
					Symbol:   data.Symbol,
					Price:    price,
					Quantity: qty,
					Side:     flow.TradeSide(data.Side.String()),
					At:       data.Timestamp,
				}); err != nil {
					signal.Error(err)
					continue
				}

				var (
					input equation.BookQualityInput
					ready bool
					err   error
				)

				if len(bidOrders)+len(askOrders) > 0 {
					input, ready, _, err = signal.sample.MeasureLevel3Snapshot(quality.Level3Input{
						Symbol: data.Symbol,
						Bids:   bidOrders,
						Asks:   askOrders,
					})
				} else {
					input, ready, _, err = signal.sample.MeasureBook(flow.BookInput{
						Symbol: data.Symbol,
						Bids:   bids,
						Asks:   asks,
					})
				}

				if err != nil {
					signal.Error(err)
					continue
				}

				if !ready {
					continue
				}

				outcome, err := signal.process.Measure(input)
				if err != nil {
					signal.Error(err)
					continue
				}

				// Publish neutrals too — resonance holds until every Map() label
				// has been seen, and Category 0 still carries vacuum_score.
				mu.Lock()
				envelope.Toxicity[data.Symbol] = &outcome
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Toxicity[focus]; ok {
		ui.Focus("toxicity", focus, reading)
	}

	return envelope
}

func resting(side *market.BookSide) ([]flow.BookLevel, []quality.OrderEvent) {
	if side == nil {
		return nil, nil
	}

	levels := make([]flow.BookLevel, 0, len(side.Levels))
	var events []quality.OrderEvent

	for _, level := range side.Levels {
		if level == nil || level.Price == nil || level.Quantity == nil || level.Price.Sign() <= 0 {
			continue
		}

		price := level.Price.InexactFloat64()
		levels = append(levels, flow.BookLevel{
			Price:    price,
			Quantity: level.Quantity.InexactFloat64(),
		})

		for _, order := range level.Queue() {
			if order == nil || order.ID == "" || order.Quantity == nil || order.Quantity.Sign() <= 0 {
				continue
			}

			events = append(events, quality.OrderEvent{
				Event:    "modify",
				OrderID:  order.ID,
				Price:    price,
				Quantity: order.Quantity.InexactFloat64(),
			})
		}
	}

	return levels, events
}
