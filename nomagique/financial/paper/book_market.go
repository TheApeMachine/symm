package paper

import (
	"bytes"
	"encoding/json"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
)

/* identify selects a carried quote for a normalized market record. */
func (server *BookServer) identify(encoded []byte) error {
	encoded = bytes.TrimSpace(encoded)

	if len(encoded) == 0 {
		return nil
	}

	if encoded[0] == '[' {
		var entries []json.RawMessage

		if err := json.Unmarshal(encoded, &entries); err != nil {
			return errnie.Error(err)
		}

		if len(entries) != 1 {
			return errnie.Error(errnie.Err(errnie.Validation, "paper book: one observation requires one market record", nil))
		}
		encoded = entries[0]
	}
	var record struct {
		Symbol string `json:"symbol"`
	}

	if err := json.Unmarshal(encoded, &record); err != nil {
		return errnie.Error(err)
	}
	server.symbol = record.Symbol
	return nil
}

/* market publishes typed levels without a JSON round trip or another book owner. */
func (server *BookServer) market(result Market, held *book.Book) error {
	if err := result.SetSymbol(server.symbol); err != nil {
		return errnie.Error(err)
	}
	result.SetUpdated(server.updated)
	var bidCount, askCount int32
	for level := held.BestBid(); level != nil; level = level.Lower {
		bidCount++
	}
	for level := held.BestAsk(); level != nil; level = level.Higher {
		askCount++
	}
	bids, err := result.NewBids(bidCount)

	if err != nil {
		return errnie.Error(err)
	}
	asks, err := result.NewAsks(askCount)

	if err != nil {
		return errnie.Error(err)
	}
	index := 0
	for level := held.BestBid(); level != nil; level = level.Lower {
		if err := bids.At(index).SetPrice(level.Price.String()); err != nil {
			return errnie.Error(err)
		}

		if err := bids.At(index).SetQuantity(level.Quantity.String()); err != nil {
			return errnie.Error(err)
		}
		index++
	}
	index = 0
	for level := held.BestAsk(); level != nil; level = level.Higher {
		if err := asks.At(index).SetPrice(level.Price.String()); err != nil {
			return errnie.Error(err)
		}

		if err := asks.At(index).SetQuantity(level.Quantity.String()); err != nil {
			return errnie.Error(err)
		}
		index++
	}
	if server.updated {
		return server.orders(result, held)
	}
	return nil
}

/* orders publishes the canonical venue queue, retaining order identity and priority. */
func (server *BookServer) orders(result Market, held *book.Book) error {
	var count int32
	for _, direction := range []bool{true, false} {
		level := held.BestAsk()
		if direction {
			level = held.BestBid()
		}
		for level != nil {
			for range level.Queue() {
				count++
			}
			if direction {
				level = level.Lower
				continue
			}
			level = level.Higher
		}
	}
	orders, err := result.NewOrders(count)
	if err != nil {
		return errnie.Error(err)
	}
	index := 0
	for _, bid := range []bool{true, false} {
		level := held.BestAsk()
		if bid {
			level = held.BestBid()
		}
		var rank uint32
		for level != nil {
			for _, order := range level.Queue() {
				entry := orders.At(index)
				if err := entry.SetId(order.ID); err != nil {
					return errnie.Error(err)
				}
				if err := entry.SetPrice(order.LimitPrice.String()); err != nil {
					return errnie.Error(err)
				}
				if err := entry.SetQuantity(order.Quantity.String()); err != nil {
					return errnie.Error(err)
				}
				entry.SetBid(bid)
				entry.SetRank(rank)
				rank++
				index++
			}
			if bid {
				level = level.Lower
				continue
			}
			level = level.Higher
		}
	}
	return nil
}
