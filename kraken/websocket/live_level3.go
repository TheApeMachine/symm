package websocket

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

/*
SubscribeLevel3 subscribes this authenticated transport to L3 via the SDK
SubL3 helper, matching Kraken's official BookManager example.
*/
func (live *Live) SubscribeLevel3(symbols []string, depth int) error {
	return live.client.SubL3(symbols, depth)
}

/*
updateLevel3 feeds one websocket message into the SDK BookManager under the
write lease so PeekBook readers never range Side.Levels mid-mutation.
*/
func (live *Live) updateLevel3(
	event *callback.Event[*sdkkraken.WebSocketMessage],
) error {
	if !live.isLevel3 || live.books == nil || event == nil {
		return nil
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	return live.books.Update(event)
}

/*
peekBook calls fn while holding the Level3 read lease for this transport.
*/
func (live *Live) peekBook(symbol string, fn func(*book.Book)) bool {
	if live == nil || live.books == nil || fn == nil || symbol == "" {
		return false
	}

	live.bookMu.RLock()
	defer live.bookMu.RUnlock()

	symbolBook := live.books.GetBook(symbol)

	if symbolBook == nil {
		return false
	}

	fn(symbolBook)

	return true
}

/*
ApplyLevel3 feeds one raw Level3 websocket payload through the write lease.
*/
func (live *Live) ApplyLevel3(payload []byte) error {
	if live == nil {
		return nil
	}

	if len(payload) == 0 {
		return errnie.Err(
			errnie.Validation,
			"websocket: level3 payload is empty",
			nil,
		)
	}

	return live.updateLevel3(&callback.Event[*sdkkraken.WebSocketMessage]{
		Data: sdkkraken.NewWebSocketMessage(payload),
	})
}

/*
invalidateLevel3Book recreates affected SDK books after a failed apply so a
corrupt book cannot stay on the read path.
*/
func (live *Live) invalidateLevel3Book(raw []byte) {
	if !live.isLevel3 || live.books == nil || len(raw) == 0 {
		return
	}

	frame := kraken.NewLevel3(raw)

	if len(frame.Data) == 0 {
		return
	}

	depth := viper.GetInt("market.l3_depth")

	if depth <= 0 {
		depth = 10
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	for _, data := range frame.Data {
		if data.Symbol == "" {
			continue
		}

		managed := live.books.CreateBook(data.Symbol, depth)
		managed.EnableMaxDepth = false
		managed.NoBookCrossing = false
	}
}

/*
SeedTouch installs a two-sided L3 touch for symbol under the write lease so
toxicity harness tests can PeekBook without checksummed fixture replay.
*/
func (live *Live) SeedTouch(
	symbol string,
	bid *decimal.Decimal,
	ask *decimal.Decimal,
	quantity *decimal.Decimal,
	at time.Time,
) {
	if live == nil || live.books == nil || symbol == "" || bid == nil || ask == nil {
		return
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	symbolBook := live.books.GetBook(symbol)

	if symbolBook == nil {
		symbolBook = live.books.CreateBook(symbol, 10)
		symbolBook.EnableMaxDepth = false
		symbolBook.NoBookCrossing = false
	}

	symbolBook.Update(&book.UpdateOptions{
		Direction: book.Bid,
		ID:        "seed-bid",
		Price:     bid,
		Quantity:  quantity,
		Timestamp: at,
	})

	symbolBook.Update(&book.UpdateOptions{
		Direction: book.Ask,
		ID:        "seed-ask",
		Price:     ask,
		Quantity:  quantity,
		Timestamp: at,
	})
}
