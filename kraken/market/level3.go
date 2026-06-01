package market

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
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

var level3TokenSource Level3TokenSource

/*
SetLevel3TokenSource enables authenticated L3 market data. Pass nil to disable.
*/
func SetLevel3TokenSource(source Level3TokenSource) {
	level3TokenSource = source
}

/*
Level3Available reports whether authenticated L3 market data is configured.
*/
func Level3Available() bool {
	return level3TokenSource != nil
}

/*
NewLevel3Subscription returns per-order book events when credentials are configured.
*/
func NewLevel3Subscription(
	ctx context.Context, depth int, symbols ...string,
) <-chan *Level3Update {
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

	out := make(chan *Level3Update, 128)

	client := errnie.Does(func() (*kraken.Client, error) {
		return kraken.NewClient(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	for _, batch := range symbolBatches(symbols) {
		if err := client.Send(public.Level3Channel, public.Subscription{
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

	for msg := range errnie.Does(func() (<-chan *public.SocketMessage, error) {
		stream, err := client.Stream(public.Level3Channel)

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

		var level3 Level3Update

		if err := sonic.Unmarshal(msg.Data, &level3); err != nil {
			errnie.Error(err)
			continue
		}

		out <- &level3
	}

	return out
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
