package exhaust

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Book ingests public book rows into the shared exhaust sample.
*/
type Book struct {
	ctx        context.Context
	cancel     context.CancelFunc
	api        *websocket.API
	instrument *broker.Instrument
	cache      *types.MarketFeed[kraken.BookData]
}

func NewBook(
	ctx context.Context, api *websocket.API, instrument *broker.Instrument,
) *Book {
	ctx, cancel := context.WithCancel(ctx)

	book := &Book{
		ctx:        ctx,
		cancel:     cancel,
		api:        api,
		instrument: instrument,
		cache: types.NewMarketFeed[kraken.BookData](
			viper.GetInt("signals.feed_timeline_capacity"),
			viper.GetInt("signals.feed_track_capacity"),
		),
	}

	book.api.On("book", book.On)
	return book
}

/*
On decodes an inbound market-data message and retains its relevant rows so
measurement uses the authoritative event stream.
*/
func (book *Book) On(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewBook(data)

	if len(frame.Data) == 0 {
		return
	}

	for index := range frame.Data {
		frame.Data[index].PriceIncrement = book.increment(frame.Data[index].Symbol)
	}

	for _, data := range frame.Data {
		if err := book.cache.Observe(data.Symbol, data.Timestamp, data); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"exhaust: book observation failed",
				err,
			))
			return
		}
	}
}

/*
increment resolves the exchange price increment for symbol, defaulting to a
zero decimal (skipped downstream by the existing PriceIncrement.Sign() guard)
until instrument metadata for that symbol has arrived.
*/
func (book *Book) increment(symbol string) decimal.Decimal {
	if book.instrument == nil {
		return *decimal.NewFromInt64(0)
	}

	pair, err := book.instrument.Pair(symbol)

	if err != nil {
		return *decimal.NewFromInt64(0)
	}

	return pair.Increment()
}

/*
levels converts exchange decimals into the tick-aware book levels required by
the decay sample, preserving the instrument's authoritative price increment.
*/
func (book *Book) levels(
	row kraken.BookData,
) ([]flow.BookLevel, []flow.BookLevel, error) {
	bids := make([]flow.BookLevel, 0, len(row.Bids))
	asks := make([]flow.BookLevel, 0, len(row.Asks))

	for _, level := range row.Bids {
		tick, err := kraken.PriceTick(level.Price, row.PriceIncrement)

		if err != nil {
			return nil, nil, err
		}

		bids = append(bids, flow.BookLevel{
			Price:    level.Price.Float64(),
			Ticks:    tick,
			Quantity: level.Qty,
		})
	}

	for _, level := range row.Asks {
		tick, err := kraken.PriceTick(level.Price, row.PriceIncrement)

		if err != nil {
			return nil, nil, err
		}

		asks = append(asks, flow.BookLevel{
			Price:    level.Price.Float64(),
			Ticks:    tick,
			Quantity: level.Qty,
		})
	}

	return bids, asks, nil
}
