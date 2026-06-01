package market

import (
	"context"
	"hash/crc32"
	"sort"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/public"
)

// BookSnapshot is the envelope type tag for a full L2 book frame after subscribe.
const BookSnapshot = "snapshot"

const bookChecksumLevels = 10

/*
BookParams is the Kraken WebSocket v2 subscribe payload for the book channel.
*/
type BookParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth"`
	Snapshot bool     `json:"snapshot"`
}

/*
BookLevel is one price level in an L2 book snapshot or update.
*/
type BookLevel struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

/*
Book is one L2 order book snapshot or update from the public book WebSocket feed.

Kraken delivers an initial snapshot then incremental updates; each frame carries
bids and asks with aggregated quantity per price, a CRC32 checksum over the top
ten levels per side, and an RFC3339 timestamp. Type records the envelope tag
(snapshot vs update) from the channel message, not the data payload.
*/
type Book struct {
	Symbol    string      `json:"symbol"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
	Checksum  int64       `json:"checksum"`
	Timestamp string      `json:"timestamp"`
	Type      string      `json:"-"`
}

/*
SetEnvelopeType records the channel envelope tag (snapshot or update).
*/
func (book *Book) SetEnvelopeType(kind string) {
	book.Type = kind
}

/*
IsSnapshot reports whether this frame is a full book snapshot.
*/
func (book *Book) IsSnapshot() bool {
	return book.Type == BookSnapshot
}

/*
Fold merges one Kraken book frame into the receiver per the v2 book channel guide.

Snapshots replace bids and asks; updates merge changed levels and remove entries
with qty zero, then truncate to depth.
*/
func (book *Book) Fold(update Book, depth int) {
	if depth <= 0 {
		depth = bookChecksumLevels
	}

	if update.IsSnapshot() {
		book.Symbol = update.Symbol
		book.Bids = book.cloneLevels(update.Bids)
		book.Asks = book.cloneLevels(update.Asks)
		book.truncate(depth)

		return
	}

	book.Bids = book.mergeBookSide(book.Bids, update.Bids, false)
	book.Asks = book.mergeBookSide(book.Asks, update.Asks, true)
	book.truncate(depth)
}

/*
ComputedChecksum returns the Kraken WebSocket v2 CRC32 for this frame's levels.
*/
func (book Book) ComputedChecksum() int64 {
	var builder strings.Builder

	for index := range min(bookChecksumLevels, len(book.Asks)) {
		builder.WriteString(book.checksumField(book.Asks[index].Price))
		builder.WriteString(book.checksumField(book.Asks[index].Qty))
	}

	for index := range min(bookChecksumLevels, len(book.Bids)) {
		builder.WriteString(book.checksumField(book.Bids[index].Price))
		builder.WriteString(book.checksumField(book.Bids[index].Qty))
	}

	return int64(crc32.ChecksumIEEE([]byte(builder.String())))
}

func (book Book) checksumField(value float64) string {
	raw := strings.ReplaceAll(strconv.FormatFloat(value, 'f', -1, 64), ".", "")
	raw = strings.TrimLeft(raw, "0")

	if raw == "" {
		return "0"
	}

	return raw
}

func (book *Book) truncate(depth int) {
	if len(book.Bids) > depth {
		book.Bids = book.Bids[:depth]
	}

	if len(book.Asks) > depth {
		book.Asks = book.Asks[:depth]
	}
}

func (book *Book) cloneLevels(levels []BookLevel) []BookLevel {
	if len(levels) == 0 {
		return nil
	}

	out := make([]BookLevel, len(levels))
	copy(out, levels)

	return out
}

func (book *Book) mergeBookSide(existing, delta []BookLevel, askSide bool) []BookLevel {
	byPrice := make(map[float64]float64, len(existing)+len(delta))

	for _, level := range existing {
		if level.Qty > 0 {
			byPrice[level.Price] = level.Qty
		}
	}

	for _, level := range delta {
		if level.Qty <= 0 {
			delete(byPrice, level.Price)

			continue
		}

		byPrice[level.Price] = level.Qty
	}

	return book.levelsFromMap(byPrice, askSide)
}

func (book *Book) levelsFromMap(byPrice map[float64]float64, askSide bool) []BookLevel {
	levels := make([]BookLevel, 0, len(byPrice))

	for price, qty := range byPrice {
		levels = append(levels, BookLevel{Price: price, Qty: qty})
	}

	sort.Slice(levels, func(left, right int) bool {
		if askSide {
			return levels[left].Price < levels[right].Price
		}

		return levels[left].Price > levels[right].Price
	})

	return levels
}

/*
NewBookSubscription returns a channel of L2 book snapshots and updates for symbols
at depth.
*/
func NewBookSubscription(
	ctx context.Context, depth int, symbols ...string,
) <-chan *Book {
	if depth <= 0 {
		depth = 10
	}

	out := make(chan *Book, 128)

	client := errnie.Does(func() (*kraken.Client, error) {
		return kraken.NewClient(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	if err := client.Send(public.BookChannel, public.Subscription{
		Method: public.MethodSubscribe,
		Params: BookParams{
			Channel:  public.BookChannel,
			Symbol:   symbols,
			Depth:    depth,
			Snapshot: true,
		},
	}); err != nil {
		errnie.Error(err)
	}

	for msg := range errnie.Does(func() (<-chan *public.SocketMessage, error) {
		stream, err := client.Stream(public.BookChannel)

		if err != nil {
			return nil, err
		}

		return stream, nil
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value() {
		if msg == nil {
			continue
		}

		var book Book

		if err := sonic.Unmarshal(msg.Data, &book); err != nil {
			errnie.Error(err)
			continue
		}

		book.SetEnvelopeType(msg.Type)
		out <- &book
	}

	return out
}
