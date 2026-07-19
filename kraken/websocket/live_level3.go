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

// level3QueueDepth buffers a burst of book frames between the socket reader and
// the FIFO apply worker. It absorbs jitter without unbounded memory growth; a
// sustained overflow applies natural backpressure on the reader rather than
// silently dropping frames, since a dropped frame breaks the L3 checksum chain.
const level3QueueDepth = 1024

/*
SubscribeLevel3 subscribes this authenticated transport to L3 via the SDK
SubL3 helper, matching Kraken's official BookManager example.
*/
func (live *Live) SubscribeLevel3(symbols []string, depth int) error {
	return live.client.SubL3(symbols, depth)
}

/*
enqueueLevel3 copies one raw L3 frame onto the FIFO worker queue so the socket
reader never applies book updates itself. The copy is required because the SDK
reader reuses its receive buffer once the callback returns. A full queue blocks
the reader (backpressure) but never drops a frame, and shutdown unblocks through
the transport context.
*/
func (live *Live) enqueueLevel3(raw []byte) {
	if live == nil || live.level3Queue == nil || len(raw) == 0 {
		return
	}

	frame := append([]byte(nil), raw...)

	select {
	case live.level3Queue <- frame:
	case <-live.ctx.Done():
	}
}

/*
drainLevel3 is the single FIFO worker that applies queued L3 frames under the
book write lease, preserving arrival order so checksums stay valid. A failed
apply invalidates the affected books, matching the former reader behaviour. It
exits when the transport context is cancelled on Close.
*/
func (live *Live) drainLevel3() {
	for {
		select {
		case <-live.ctx.Done():
			return
		case raw := <-live.level3Queue:
			event := &callback.Event[*sdkkraken.WebSocketMessage]{
				Data: sdkkraken.NewWebSocketMessage(raw),
			}

			if err := live.updateLevel3(event); err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"websocket: level3 book update failed: "+err.Error(),
					err,
				))
				live.invalidateLevel3Book(raw)
			}
		}
	}
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
