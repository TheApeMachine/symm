package market

import (
	"context"

	"github.com/bytedance/sonic"
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
NewBookSubscription opens the book channel at depth and forwards unmarshaled rows to recv.
It blocks until ctx is canceled or the socket closes.
*/
func NewBookSubscription(
	ctx context.Context,
	recv chan *BookUpdate,
	depth int,
	symbols ...string,
) {
	if depth <= 0 {
		depth = 10
	}

	ws := errnie.Does(func() (*public.WebSocket, error) {
		return public.NewWebSocket(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	messages := errnie.Does(func() (chan *public.SocketMessage, error) {
		return ws.Generate(public.BookChannel)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	for message := range messages {
		var rows []BookUpdate

		if err := sonic.Unmarshal(message.Data, &rows); err != nil {
			continue
		}

		for _, row := range rows {
			update := row
			recv <- &update
		}
	}
}
