package market

import (
	"context"
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/orderbook"
	"github.com/theapemachine/symm/kraken/public"
)

// BookSnapshot is the envelope "type" tag Kraken sends with a full book frame —
// the first frame after each (re)subscribe. Every other frame is an incremental
// delta and carries no snapshot tag.
const BookSnapshot = "snapshot"

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
BookLevel is one price/qty level in an L2 book snapshot or delta. It keeps the exact
numeric text the exchange sent alongside the parsed float64s because Kraken's book
checksum is computed over that text at the symbol's own precision.
*/
type BookLevel struct {
	Price    float64
	Qty      float64
	PriceRaw string
	QtyRaw   string
}

/*
UnmarshalJSON decodes a book level while preserving the raw numeric text. The float64
values feed the signals; the raw text feeds the checksum, which a reformatted float64
would not reproduce (trailing zeros encode precision and would be lost).
*/
func (level *BookLevel) UnmarshalJSON(data []byte) error {
	var wire struct {
		Price json.Number `json:"price"`
		Qty   json.Number `json:"qty"`
	}

	if err := sonic.Unmarshal(data, &wire); err != nil {
		return err
	}

	price, err := wire.Price.Float64()

	if err != nil {
		return err
	}

	qty, err := wire.Qty.Float64()

	if err != nil {
		return err
	}

	level.Price = price
	level.Qty = qty
	level.PriceRaw = wire.Price.String()
	level.QtyRaw = wire.Qty.String()

	return nil
}

/*
BookUpdate is one L2 order book snapshot or delta from the public book feed.

The live aggregated L2 order book: total resting size at each price level on both
sides, delivered as an initial snapshot then checksum-verified deltas. It shows
where liquidity is stacked and how it shifts in real time, and the checksum proves
the locally maintained book still matches the exchange exactly. Kind carries the
envelope tag ("snapshot" vs delta) so a consumer maintaining a local book knows
whether to replace it or fold the frame in.
*/
type BookUpdate struct {
	Symbol    string      `json:"symbol"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
	Checksum  int64       `json:"checksum"`
	Timestamp string      `json:"timestamp"`
	Kind      string      `json:"-"`
}

/*
SetEnvelopeType records the channel envelope tag so the consumer can tell a fresh
book from a delta. Populated by the public Stream, not the data payload.
*/
func (update *BookUpdate) SetEnvelopeType(kind string) {
	update.Kind = kind
}

/*
IsSnapshot reports whether this frame is a full book snapshot — the first frame and
every frame after a resubscribe — rather than an incremental delta.
*/
func (update *BookUpdate) IsSnapshot() bool {
	return update.Kind == BookSnapshot
}

/*
BidLevels and AskLevels convert the wire levels to maintained-book levels, carrying
the raw text through for checksum verification.
*/
func (update *BookUpdate) BidLevels() []orderbook.Level {
	return toBookLevels(update.Bids)
}

func (update *BookUpdate) AskLevels() []orderbook.Level {
	return toBookLevels(update.Asks)
}

func toBookLevels(levels []BookLevel) []orderbook.Level {
	out := make([]orderbook.Level, len(levels))

	for index, level := range levels {
		out[index] = orderbook.Level{
			Price:    level.Price,
			Qty:      level.Qty,
			PriceRaw: level.PriceRaw,
			QtyRaw:   level.QtyRaw,
		}
	}

	return out
}

/*
NewBookSubscription returns a channel of L2 book snapshots and deltas for symbols
at depth. All callers share one upstream connection via the book feed. The
caller's ctx detaches it from the shared feed; the upstream keeps running.
*/
func NewBookSubscription(
	ctx context.Context, depth int, symbols ...string,
) <-chan *BookUpdate {
	return bookFeed.subscribe(ctx, subscriptionSpec{
		symbols: symbols,
		depth:   depth,
	})
}

// dialBook opens one upstream connection to the public L2 book channel.
func dialBook(
	ctx context.Context, depth int, symbols []string,
) <-chan *BookUpdate {
	depth = validBookDepth(depth)

	ws, err := public.NewWebSocket(ctx)

	if err != nil {
		errnie.Error(err)
		return closed[BookUpdate]()
	}

	if err := ws.Connect(public.WebSocketURL, public.BookChannel); err != nil {
		errnie.Error(err)
		return closed[BookUpdate]()
	}

	for _, batch := range symbolBatches(symbols) {
		if err := ws.Send(public.BookChannel, public.Subscription{
			Method: public.MethodSubscribe,
			Params: BookParams{
				Channel:  public.BookChannel,
				Symbol:   batch,
				Depth:    depth,
				Snapshot: true,
			},
		}); err != nil {
			errnie.Error(err)
			return closed[BookUpdate]()
		}
	}

	stream, err := public.Stream[BookUpdate](ws, public.BookChannel)

	if err != nil {
		errnie.Error(err)
		return closed[BookUpdate]()
	}

	return stream
}
