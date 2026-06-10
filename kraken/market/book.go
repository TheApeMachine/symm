package market

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/types"
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

func NewBookParams(symbols []string, depth int) json.RawMessage {
	params := &BookParams{
		Channel:  "book",
		Symbol:   symbols,
		Depth:    depth,
		Snapshot: true,
	}

	raw, err := sonic.Marshal(params)

	if errnie.Error(err) != nil {
		return nil
	}

	return json.RawMessage(raw)
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
type BookUpdate struct {
	Symbol    string      `json:"symbol"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
	Checksum  int64       `json:"checksum"`
	Timestamp time.Time   `json:"timestamp"`
	Type      string      `json:"-"`
}

func (book *BookUpdate) Unmarshal(message *types.SocketMessage) error {
	if err := sonic.Unmarshal(message.Data, book); err != nil {
		return err
	}

	book.Type = message.Type

	return nil
}

type BookUpdates []*BookUpdate

func (updates *BookUpdates) Unmarshal(message *types.SocketMessage) error {
	if err := sonic.Unmarshal(message.Data, updates); err != nil {
		return err
	}

	for _, update := range *updates {
		if update == nil {
			continue
		}

		update.Type = message.Type
	}

	return nil
}
