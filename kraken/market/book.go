package market

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

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
BookLevel is one price/qty level in an L2 book snapshot or delta.
*/
type BookLevel struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

/*
BookUpdate is one L2 order book snapshot or delta from the public book feed.

The live aggregated L2 order book: total resting size at each price level on both
sides, delivered as an initial snapshot then checksum-verified deltas. It shows
where liquidity is stacked and how it shifts in real time, and the checksum proves
the locally maintained book still matches the exchange exactly.
*/
type BookUpdate struct {
	Symbol    string      `json:"symbol"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
	Checksum  int64       `json:"checksum"`
	Timestamp string      `json:"timestamp"`
}

/*
NewBookSubscription returns a channel of L2 book snapshots and deltas for symbols
at depth. All callers share one upstream connection via the book feed. The
caller's ctx detaches it from the shared feed; the upstream keeps running.
*/
func NewBookSubscription(
	ctx context.Context, depth int, symbols ...string,
) <-chan *BookUpdate {
	return bookFeed.subscribe(ctx, func() <-chan *BookUpdate {
		return dialBook(context.Background(), depth, symbols)
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

	if err := ws.Send(public.BookChannel, public.Subscription{
		Method: public.MethodSubscribe,
		Params: BookParams{
			Channel:  public.BookChannel,
			Symbol:   symbols,
			Depth:    depth,
			Snapshot: true,
		},
	}); err != nil {
		errnie.Error(err)
		return closed[BookUpdate]()
	}

	stream, err := public.Stream[BookUpdate](ws, public.BookChannel)

	if err != nil {
		errnie.Error(err)
		return closed[BookUpdate]()
	}

	return stream
}
