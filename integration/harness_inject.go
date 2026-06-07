package integration

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
)

/*
InjectTrade publishes one trade frame onto the shared raw bus after replay.
*/
func (harness *Harness) InjectTrade(trade market.TradeUpdate) {
	if trade.Timestamp.IsZero() {
		trade.Timestamp = time.Now().UTC()
	}

	raw, err := qpool.NewBroadcastGroup(harness.ctx, "raw", 10*time.Millisecond)
	if err != nil {
		return
	}

	envelope, err := marketTradeEnvelope(trade)

	if err != nil {
		return
	}

	raw.Send(&qpool.QValue[any]{
		Type:  public.TradesChannel,
		Value: envelope,
	})
}

/*
InjectTicker publishes one ticker frame onto the shared raw bus after replay.
*/
func (harness *Harness) InjectTicker(ticker market.TickerUpdate) {
	raw, err := qpool.NewBroadcastGroup(harness.ctx, "raw", 10*time.Millisecond)
	if err != nil {
		return
	}

	if ticker.Timestamp == "" {
		ticker.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	rawData, err := sonic.Marshal([]market.TickerUpdate{ticker})

	if err != nil {
		return
	}

	raw.Send(&qpool.QValue[any]{
		Type: public.TickerChannel,
		Value: map[string]any{
			"channel": public.TickerChannel,
			"type":    "update",
			"data":    json.RawMessage(rawData),
		},
	})
}

/*
InjectBook publishes one book snapshot onto the shared raw bus.
*/
func (harness *Harness) InjectBook(book market.Book) {
	raw, err := qpool.NewBroadcastGroup(harness.ctx, "raw", 10*time.Millisecond)
	if err != nil {
		return
	}

	if book.Timestamp == "" {
		book.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	book.SetEnvelopeType(market.BookSnapshot)

	rawData, err := sonic.Marshal([]market.Book{book})

	if err != nil {
		return
	}

	raw.Send(&qpool.QValue[any]{
		Type: public.BookChannel,
		Value: map[string]any{
			"channel": public.BookChannel,
			"type":    market.BookSnapshot,
			"data":    json.RawMessage(rawData),
		},
	})
}

func marketTradeEnvelope(trade market.TradeUpdate) (map[string]any, error) {
	raw, err := sonic.Marshal([]market.TradeUpdate{trade})

	if err != nil {
		return nil, err
	}

	return map[string]any{
		"channel": public.TradesChannel,
		"type":    "update",
		"data":    json.RawMessage(raw),
	}, nil
}
