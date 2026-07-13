package exhaust

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/trader"
)

/*
Book ingests public book rows into the shared exhaust sample.
*/
type Book struct {
	ctx        context.Context
	cancel     context.CancelFunc
	api        *websocket.API
	instrument *trader.Instrument
	cache      []kraken.BookData
}

func NewBook(
	ctx context.Context, api *websocket.API, instrument *trader.Instrument,
) *Book {
	ctx, cancel := context.WithCancel(ctx)

	book := &Book{
		ctx:        ctx,
		cancel:     cancel,
		api:        api,
		instrument: instrument,
		cache:      []kraken.BookData{},
	}

	book.api.On("book", book.On)
	return book
}

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

	book.cache = append(book.cache, frame.Data...)
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
