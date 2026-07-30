package websocket

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
newLevel3Consumer creates the ordinary production book processor for one Conn.
*/
func newLevel3Consumer(
	ctx context.Context,
	symbols []string,
	depth int,
) *Live {
	live := New(ctx, nil, true, Level3WebSocketURL)
	live.symbols = append([]string(nil), symbols...)
	live.ensureLevel3Books(symbols, depth)
	live.status = types.READY

	return live
}

// level3QueueDepth buffers a burst of book frames between the socket reader and
// the FIFO apply worker. It absorbs jitter without unbounded memory growth; a
// sustained overflow applies natural backpressure on the reader rather than
// silently dropping frames, since a dropped frame breaks the L3 checksum chain.
const level3QueueDepth = 1024

/*
SubscribeLevel3 ensures each symbol has a BookManager entry, then subscribes via
the SDK SubL3 helper. Books must exist before the first snapshot/update arrives;
Kraken's example creates them from the outbound subscribe (OnSent) — we do both.
*/
func (live *Live) SubscribeLevel3(symbols []string, depth int) error {
	if live == nil || live.client == nil {
		return errnie.Err(errnie.NotFound, "websocket: level3 transport unavailable", nil)
	}

	if depth <= 0 {
		depth = 10
	}

	live.ensureLevel3Books(symbols, depth)

	return live.client.SubL3(symbols, depth)
}

/*
ensureLevel3Books creates missing SDK books under the write lease so inbound
frames never hit "not found in library" before the subscribe ack path runs.
*/
func (live *Live) ensureLevel3Books(symbols []string, depth int) {
	if live == nil || live.books == nil || depth <= 0 {
		return
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	for _, symbol := range symbols {
		if symbol == "" || live.books.GetBook(symbol) != nil {
			continue
		}

		managed := live.books.CreateBook(symbol, depth)
		managed.EnableMaxDepth = false
		managed.NoBookCrossing = false
	}
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
updateLevel3 applies one websocket message through the exact-text ledger under
the SDK book write lease so PeekBook readers never range Side.Levels during
mutation.
*/
func (live *Live) updateLevel3(
	event *callback.Event[*sdkkraken.WebSocketMessage],
) error {
	if !live.isLevel3 || live.books == nil || live.level3Ledger == nil ||
		event == nil || event.Data == nil {
		return nil
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	return live.level3Ledger.Apply(live.books, event.Data.Bytes())
}

/*
ingestLevel3Sent feeds outbound websocket frames into BookManager. Kraken's L3
example creates books from the subscribe request on OnSent; without this path,
snapshots race in against an empty library.
*/
func (live *Live) ingestLevel3Sent(
	event *callback.Event[*sdkkraken.WebSocketMessage],
) {
	if live == nil || !live.isLevel3 || live.books == nil || event == nil {
		return
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	if err := live.books.Update(event); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: level3 sent-frame book ingest failed: "+err.Error(),
			err,
		))
	}
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

	event := &callback.Event[*sdkkraken.WebSocketMessage]{
		Data: sdkkraken.NewWebSocketMessage(payload),
	}

	if utils.GetString(payload, "method") == "subscribe" {
		live.ingestLevel3Sent(event)
		return nil
	}

	return live.updateLevel3(event)
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

	symbols := make([]string, 0, len(frame.Data))

	live.bookMu.Lock()

	for _, data := range frame.Data {
		if data.Symbol == "" {
			continue
		}

		delete(live.level3Ledger.orders, data.Symbol)
		live.level3Ledger.waiting[data.Symbol] = struct{}{}
		managed := live.books.CreateBook(data.Symbol, depth)
		managed.EnableMaxDepth = false
		managed.NoBookCrossing = false
		symbols = append(symbols, data.Symbol)
	}

	live.bookMu.Unlock()

	if err := live.resubscribeLevel3(symbols, depth); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: level3 resubscribe failed: "+err.Error(),
			err,
		))
	}
}

/*
resubscribeLevel3 forces a fresh private Level3 snapshot for the affected
symbols so post-invalidation updates are not applied against an empty book.
*/
func (live *Live) resubscribeLevel3(symbols []string, depth int) error {
	if live == nil || live.client == nil || len(symbols) == 0 {
		return nil
	}

	if depth <= 0 {
		depth = 10
	}

	if err := live.client.SendPrivate(map[string]any{
		"method": "unsubscribe",
		"params": map[string]any{
			"channel": "level3",
			"symbol":  symbols,
		},
	}); err != nil {
		return err
	}

	return live.SubscribeLevel3(symbols, depth)
}
