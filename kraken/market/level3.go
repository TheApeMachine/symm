package market

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Level3TokenSource supplies short-lived authenticated WebSocket tokens.
*/
type Level3TokenSource interface {
	Token(context.Context) (string, error)
}

/*
Level3Params is the Kraken WebSocket v2 subscribe payload for the level3 channel.
*/
type Level3Params struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth"`
	Snapshot bool     `json:"snapshot"`
	Token    string   `json:"token"`
}

/*
Level3OrderEvent is one add, modify, or delete event for a resting order on the L3 feed.
*/
type Level3OrderEvent struct {
	Event      string  `json:"event"`
	OrderID    string  `json:"order_id"`
	LimitPrice float64 `json:"limit_price"`
	OrderQty   float64 `json:"order_qty"`
	Timestamp  string  `json:"timestamp"`
}

/*
Level3Update is one per-order book delta from the authenticated level3 WebSocket feed.
*/
type Level3Update struct {
	Symbol    string             `json:"symbol"`
	Bids      []Level3OrderEvent `json:"bids"`
	Asks      []Level3OrderEvent `json:"asks"`
	Checksum  int64              `json:"checksum"`
	Timestamp string             `json:"timestamp"`
}

var (
	level3TokenSource Level3TokenSource
	level3Feed        *sharedFeed[Level3Update]
)

/*
SetLevel3TokenSource enables the shared authenticated L3 feed. Pass nil to disable.
*/
func SetLevel3TokenSource(source Level3TokenSource) {
	level3TokenSource = source

	if source == nil {
		level3Feed = nil

		return
	}

	level3Feed = newReliableSharedFeed(func(ctx context.Context, spec subscriptionSpec) <-chan *Level3Update {
		return dialLevel3(ctx, spec.depth, spec.symbols)
	})
}

/*
Level3Available reports whether authenticated L3 market data is configured.
*/
func Level3Available() bool {
	return level3TokenSource != nil && level3Feed != nil
}

/*
NewLevel3Subscription returns per-order book events when credentials are configured.
*/
func NewLevel3Subscription(
	ctx context.Context, depth int, symbols ...string,
) <-chan *Level3Update {
	if level3Feed == nil {
		return closed[Level3Update]()
	}

	return level3Feed.subscribe(ctx, subscriptionSpec{
		symbols: symbols,
		depth:   depth,
	})
}

func dialLevel3(ctx context.Context, depth int, symbols []string) <-chan *Level3Update {
	if level3TokenSource == nil {
		return closed[Level3Update]()
	}

	if depth <= 0 {
		depth = 10
	}

	token, err := level3TokenSource.Token(ctx)

	if err != nil {
		errnie.Error(err)
		return closed[Level3Update]()
	}

	ws, err := public.NewWebSocket(ctx)

	if err != nil {
		errnie.Error(err)
		return closed[Level3Update]()
	}

	if err := ws.Connect(public.WebSocketL3URL, public.Level3Channel); err != nil {
		errnie.Error(err)
		return closed[Level3Update]()
	}

	for _, batch := range symbolBatches(symbols) {
		if err := ws.Send(public.Level3Channel, public.Subscription{
			Method: public.MethodSubscribe,
			Params: Level3Params{
				Channel:  public.Level3Channel,
				Symbol:   batch,
				Depth:    depth,
				Snapshot: true,
				Token:    token,
			},
		}); err != nil {
			errnie.Error(err)
			return closed[Level3Update]()
		}
	}

	stream, err := public.Stream[Level3Update](ws, public.Level3Channel)

	if err != nil {
		errnie.Error(err)
		return closed[Level3Update]()
	}

	return stream
}

/*
Level3EventTime parses an L3 event timestamp, falling back to now when absent.
*/
func Level3EventTime(raw string, fallback time.Time) time.Time {
	if raw == "" {
		return fallback
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, raw)

		if err == nil {
			return parsed
		}
	}

	return fallback
}
