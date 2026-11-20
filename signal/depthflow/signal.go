package depthflow

import (
	"context"
	"math"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
bookflowHistory is the Bookflow sample depth — long enough for thin/loaded to
see pressure shift across ticks without unbounded allocator growth per symbol.
*/
const bookflowHistory = 128

/*
Signal is the DepthFlow disruptor stage — outcomes land on envelope.DepthFlow and
Map() feeds logic/resonance Features (thin_score, loaded_score) for Model story gates.
*/
type Signal struct {
	*runtime.System
	books      *broker.Book
	instrument *broker.Instrument
	price      *broker.Price
	sample     *flow.Sample
	process    *equation.Bookflow
}

/*
NewSignal needs L3 books and instrument ticks so depth snapshots align with
venue qty steps — without both, depthflow cannot publish the labels resonance expects.
Optional price caches Cont event OFI for profit-only tranche pacing.
*/
func NewSignal(
	ctx context.Context,
	books *broker.Book,
	instrument *broker.Instrument,
	price *broker.Price,
) *Signal {
	sample, err := flow.NewSample(bookflowHistory)

	signal := &Signal{
		System:     runtime.NewSystem(ctx, "depthflow"),
		books:      books,
		instrument: instrument,
		price:      price,
		sample:     sample,
		process:    equation.NewBookflow(),
	}

	if err != nil {
		signal.Error(errnie.Err(
			errnie.Validation,
			"depthflow: new sample",
			err,
		))
	}

	return signal
}

/*
Step snapshots full depth per print then folds trades — thin vs loaded are hold
features; partial publishes keep resonance from waiting on a quiet symbol.
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
					bids     []flow.BookLevel
					asks     []flow.BookLevel
					tickSize float64
					haveBook bool
				)

				signal.books.Book(data.Symbol, func(book *market.Book) {
					pair := signal.instrument.Pair(data.Symbol)

					if pair.TickSize == nil {
						return
					}

					tickSize = pair.TickSize.InexactFloat64()

					if tickSize <= 0 {
						return
					}

					bids = make([]flow.BookLevel, 0, len(book.Bids.Levels))

					for _, level := range book.Bids.Levels {
						levelPrice := level.Price.InexactFloat64()
						bids = append(bids, flow.BookLevel{
							Price:    levelPrice,
							Quantity: level.Quantity.InexactFloat64(),
							Ticks:    int64(math.Round(levelPrice / tickSize)),
						})
					}

					asks = make([]flow.BookLevel, 0, len(book.Asks.Levels))

					for _, level := range book.Asks.Levels {
						levelPrice := level.Price.InexactFloat64()
						asks = append(asks, flow.BookLevel{
							Price:    levelPrice,
							Quantity: level.Quantity.InexactFloat64(),
							Ticks:    int64(math.Round(levelPrice / tickSize)),
						})
					}

					haveBook = true
				})

				if !haveBook {
					continue
				}

				input, ready, _, err := signal.sample.MeasureBook(flow.BookInput{
					Symbol:   data.Symbol,
					TickSize: tickSize,
					Bids:     bids,
					Asks:     asks,
				})

				if err != nil {
					signal.Error(err)
					continue
				}

				if !ready {
					continue
				}

				input, ready, _, err = signal.sample.MeasureTrade(flow.TradeInput{
					Symbol:   data.Symbol,
					Price:    price,
					Quantity: qty,
					Side:     flow.TradeSide(data.Side.String()),
					At:       data.Timestamp,
				})

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

				if signal.price != nil {
					signal.price.NoteOFI(data.Symbol, outcome.EventOFI)
				}

				// Publish not-ready/zero readings too — resonance holds until every
				// Map() label has been seen (thin_score / loaded_score).
				mu.Lock()
				envelope.DepthFlow[data.Symbol] = &outcome
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.DepthFlow[focus]; ok {
		ui.Focus("depthflow", focus, reading)
	}

	return envelope
}
