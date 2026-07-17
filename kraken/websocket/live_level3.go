package websocket

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/theapemachine/errnie"
)

/*
SubscribeLevel3 sends the configured depth explicitly because the SDK's depth
argument is not included in its level3 subscription payload.
*/
func (live *Live) SubscribeLevel3(symbols []string, depth int) error {
	return live.client.SubPrivate("level3", map[string]any{
		"params": map[string]any{
			"symbol": symbols,
			"depth":  depth,
		},
	})
}

/*
updateLevel3 applies one complete websocket message before truncating affected
books, preserving Kraken's atomic L3 message boundary. The write lease excludes
PeekBook readers so Side.Levels is never ranged while the book mutates.
*/
func (live *Live) updateLevel3(
	event *callback.Event[*kraken.WebSocketMessage],
) error {
	if !live.isLevel3 || live.books == nil || live.level3 == nil {
		return nil
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	return live.level3.applyFrame(live.books, event.Data.Bytes())
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

	return live.updateLevel3(&callback.Event[*kraken.WebSocketMessage]{
		Data: kraken.NewWebSocketMessage(payload),
	})
}

/*
SeedTouchDecimals installs a two-sided L3 touch using exact decimal prices.
*/
func (live *Live) SeedTouchDecimals(
	symbol string,
	bid *decimal.Decimal,
	ask *decimal.Decimal,
	quantity float64,
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

	quantityDecimal := decimal.NewFromFloat64(quantity)
	qtyText := quantityDecimal.String()

	if live.level3 != nil {
		live.level3.remember(symbol, "seed-bid", level3WireFromText(bid.String(), qtyText))
		live.level3.remember(symbol, "seed-ask", level3WireFromText(ask.String(), qtyText))
	}

	symbolBook.Update(&book.UpdateOptions{
		Direction: book.Bid,
		ID:        "seed-bid",
		Price:     bid,
		Quantity:  quantityDecimal,
		Timestamp: at,
	})
	symbolBook.Update(&book.UpdateOptions{
		Direction: book.Ask,
		ID:        "seed-ask",
		Price:     ask,
		Quantity:  quantityDecimal,
		Timestamp: at,
	})
}

/*
SeedTouch installs a two-sided L3 touch for symbol under the write lease so
toxicity harness tests can PeekBook without checksummed fixture replay.
*/
func (live *Live) SeedTouch(
	symbol string,
	bid float64,
	ask float64,
	quantity float64,
	at time.Time,
) {
	if live == nil || live.books == nil || symbol == "" {
		return
	}

	live.SeedTouchDecimals(
		symbol,
		decimal.NewFromFloat64(bid),
		decimal.NewFromFloat64(ask),
		quantity,
		at,
	)
}
