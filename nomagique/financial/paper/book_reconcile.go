package paper

import (
	"encoding/json"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
levelReconciliation retains only the affected level's exact pre-update facts
for one SDK update batch. The SDK remains the sole owner of orders and queues.
*/
type levelReconciliation struct {
	side     *book.Side
	price    string
	quantity *decimal.Decimal
	orders   map[string]*decimal.Decimal
	scale    int64
}

/*
reconcileL3 preserves exact level totals while the SDK reconciles orders and
checks its own checksum. SDK v2.0.0 rounds Add/Sub to the receiver's scale, including
integer zero on deletes. Its synchronous update hook restores the exact aggregate
before the next record and before checksum validation; order decimals stay intact.
*/
func (server *BookServer) reconcileL3(held *book.Book, entry map[string]any) error {
	levels, err := server.reconciliation(held, entry)
	if err != nil {
		return err
	}
	observer := held.OnUpdated.Recurring(func(event *callback.Event[*book.UpdateOptions]) {
		update := event.Data
		level := levels[string(update.Direction)+":"+update.Price.String()]
		previous := level.orders[update.ID]
		incoming := update.Quantity
		scale := max(level.scale, incoming.GetScale())
		level.quantity = level.quantity.SetScale(scale)
		if previous != nil {
			level.quantity = level.quantity.Sub(previous)
		}
		level.quantity = level.quantity.Add(incoming)
		level.orders[update.ID] = incoming
		current := level.side.Levels[level.price]
		if current != nil {
			current.Quantity = level.quantity.Copy()
		}
	})
	defer held.OnUpdated.Deregister(observer)
	return server.reconcile.UpdateL3(held, entry)
}

/*
reconciliation reads the SDK's current queue once per touched price and aligns
zero-quantity deletion with its exact precision. It does not reconstruct a book.
*/
func (server *BookServer) reconciliation(held *book.Book, entry map[string]any) (map[string]*levelReconciliation, error) {
	levels := make(map[string]*levelReconciliation)
	for _, direction := range []struct {
		name string
		kind book.BookDirection
		side *book.Side
	}{{"bids", book.Bid, held.Bids}, {"asks", book.Ask, held.Asks}} {
		records, ok := entry[direction.name].([]any)
		if !ok {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "paper book: level3 side requires order records", nil))
		}
		for _, item := range records {
			record, ok := item.(map[string]any)
			if !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "paper book: level3 order must be an object", nil))
			}
			price, ok := record["limit_price"].(json.Number)
			if !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "paper book: level3 order price required", nil))
			}
			parsed, err := decimal.NewFromString(price.String())
			if err != nil {
				return nil, errnie.Error(err)
			}
			key := string(direction.kind) + ":" + parsed.String()
			level := levels[key]
			if level == nil {
				level = newLevelReconciliation(direction.side, parsed.String())
				levels[key] = level
			}

			if quantity, ok := record["order_qty"].(json.Number); ok {
				value, err := decimal.NewFromString(quantity.String())
				if err != nil {
					return nil, errnie.Error(err)
				}
				level.scale = max(level.scale, value.GetScale())
			}
		}
		// The first pass also sees more precise earlier/later orders in a snapshot.
		for _, item := range records {
			record := item.(map[string]any)
			price, err := decimal.NewFromString(record["limit_price"].(json.Number).String())
			if err != nil {
				return nil, errnie.Error(err)
			}
			level := levels[string(direction.kind)+":"+price.String()]
			if current := direction.side.Levels[level.price]; current != nil {
				current.Quantity = current.Quantity.SetScale(level.scale)
			}
			if record["event"] == "delete" {
				record["event"] = "modify"
				record["order_qty"] = json.Number(decimal.NewFromInt64(0).SetScale(level.scale).String())
			}
		}
	}
	return levels, nil
}

/* newLevelReconciliation captures the exact SDK facts needed by one level update. */
func newLevelReconciliation(side *book.Side, price string) *levelReconciliation {
	level := &levelReconciliation{side: side, price: price, quantity: decimal.NewFromInt64(0), orders: make(map[string]*decimal.Decimal)}
	current := side.Levels[price]
	if current == nil {
		return level
	}
	level.quantity = current.Quantity.Copy()
	level.scale = current.Quantity.GetScale()
	for _, order := range current.Queue() {
		level.orders[order.ID] = order.Quantity
		level.scale = max(level.scale, order.Quantity.GetScale())
	}
	return level
}
