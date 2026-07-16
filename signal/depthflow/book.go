package depthflow

import (
	"container/ring"
	"context"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Book ingests public book rows into the shared depth-flow sample.
*/
type Book struct {
	ctx        context.Context
	cancel     context.CancelFunc
	api        *websocket.API
	instrument *broker.Instrument
	cache      *sync.Map
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
		cache:      &sync.Map{},
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
		found, _ := book.cache.LoadOrStore(data.Symbol, ring.New(
			viper.GetInt("signals.feed_ring_capacity"),
		))

		track := found.(*ring.Ring)
		track.Value = data
		book.cache.Store(data.Symbol, track.Next())
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
