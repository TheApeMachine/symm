package market

import (
	"encoding/json"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/* Order describes an exact venue order in a replay fixture. */
type Order struct{ Price, Quantity, At string }

/*
Level3Frame renders a Kraken level3 frame carrying the checksum the exchange
would send for the book after it, computed by the exchange SDK's own book.
*/
func Level3Frame(kind, symbol string, before, after [2][]Order, event string) []byte {
	replica := book.New()
	records := func(direction book.BookDirection, orders []Order, emit bool) []map[string]any {
		out := make([]map[string]any, 0, len(orders))

		for _, placed := range orders {
			price, err := decimal.NewFromString(placed.Price)
			if err != nil {
				panic(err)
			}
			quantity, err := decimal.NewFromString(placed.Quantity)
			if err != nil {
				panic(err)
			}
			at, err := time.Parse(time.RFC3339, placed.At)
			if err != nil {
				panic(err)
			}
			identity := placed.At + placed.Price

			if emit && event == "delete" {
				quantity = decimal.NewFromInt64(0)
			}
			replica.Update(&book.UpdateOptions{Direction: direction, ID: identity, Price: price, Quantity: quantity, Timestamp: at})

			if !emit {
				continue
			}
			record := map[string]any{
				"order_id": identity, "limit_price": json.Number(placed.Price),
				"order_qty": json.Number(placed.Quantity), "timestamp": placed.At,
			}

			if event != "" {
				record["event"] = event
			}
			out = append(out, record)
		}
		return out
	}
	records(book.Bid, before[0], false)
	records(book.Ask, before[1], false)
	entry := map[string]any{"symbol": symbol, "bids": records(book.Bid, after[0], true), "asks": records(book.Ask, after[1], true)}
	entry["checksum"] = json.Number(replica.L3Checksum("").LocalChecksum)
	frame, err := json.Marshal(map[string]any{"channel": "level3", "type": kind, "data": []any{entry}})
	if err != nil {
		panic(err)
	}
	return frame
}
