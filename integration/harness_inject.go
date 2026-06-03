package integration

import (
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

	raw := harness.pool.CreateBroadcastGroup("raw", 10*time.Millisecond)

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
	raw := harness.pool.CreateBroadcastGroup("raw", 10*time.Millisecond)

	rawData, err := sonic.Marshal([]market.TickerUpdate{ticker})

	if err != nil {
		return
	}

	raw.Send(&qpool.QValue[any]{
		Type: public.TickerChannel,
		Value: public.SocketMessage{
			Channel: public.TickerChannel,
			Type:    "update",
			Data:    rawData,
		},
	})
}

/*
InjectBook publishes one book snapshot onto the shared raw bus.
*/
func (harness *Harness) InjectBook(book market.Book) {
	raw := harness.pool.CreateBroadcastGroup("raw", 10*time.Millisecond)

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
		Value: public.SocketMessage{
			Channel: public.BookChannel,
			Type:    market.BookSnapshot,
			Data:    rawData,
		},
	})
}

func marketTradeEnvelope(trade market.TradeUpdate) (public.SocketMessage, error) {
	raw, err := sonic.Marshal([]market.TradeUpdate{trade})

	if err != nil {
		return public.SocketMessage{}, err
	}

	return public.SocketMessage{
		Channel: public.TradesChannel,
		Type:    "update",
		Data:    raw,
	}, nil
}
