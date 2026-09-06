package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

type Book struct {
	Channel string     `json:"channel"`
	Type    string     `json:"type"`
	Data    []BookData `json:"data"`
}

type BookData struct {
	Symbol         string           `json:"symbol"`
	Type           string           `json:"type"`
	PriceIncrement *decimal.Decimal `json:"-"`
	Bids           []BookLevel      `json:"bids"`
	Asks           []BookLevel      `json:"asks"`
	Checksum       uint32           `json:"checksum"`
	Timestamp      time.Time        `json:"timestamp"`
}

type BookLevel struct {
	Price decimal.Decimal `json:"price"`
	Qty   float64         `json:"qty"`
}

func NewBook(buf []byte) *Book {
	var book Book

	if err := sonic.Unmarshal(buf, &book); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid book",
			err,
		))
	}

	for index := range book.Data {
		if book.Data[index].Type == "" {
			book.Data[index].Type = book.Type
		}
	}

	return &book
}

func (book *Book) IsSuccess() bool {
	return len(book.Data) > 0
}

func (book *Book) Action() string {
	return "book"
}

/*
ChannelUnsubscription requests Kraken stop streaming an arbitrary channel for
the given symbols. It is the shutdown counterpart to the per-channel
subscriptions: a session that is being torn down withdraws its universe from
the venue before closing the socket, so the venue is not left streaming into a
socket that is about to disappear.
*/
type ChannelUnsubscription struct {
	Channel string
	Symbols []string
}

func NewChannelUnsubscription(channel string, symbols []string) ChannelUnsubscription {
	return ChannelUnsubscription{Channel: channel, Symbols: symbols}
}

func (cu ChannelUnsubscription) MarshalJSON() ([]byte, error) {
	params := map[string]any{
		"channel": cu.Channel,
	}

	if len(cu.Symbols) > 0 {
		params["symbol"] = cu.Symbols
	}

	return sonic.Marshal(map[string]any{
		"method": "unsubscribe",
		"params": params,
	})
}

/*
Ping is the spot keepalive request. Kraken answers it on the "pong" channel,
but the write is what matters: a periodic write is what exposes a half-open
connection, since the unacknowledged retransmits eventually fail the socket.
ReqID is echoed back in the reply.
*/
type Ping struct {
	ReqID int64
}

func NewPing(reqID int64) Ping {
	return Ping{ReqID: reqID}
}

func (ping Ping) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "ping",
		"req_id": ping.ReqID,
	})
}
