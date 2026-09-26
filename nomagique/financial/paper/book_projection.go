package paper

import (
	"encoding/json"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
)

/* flow reduces the arriving mutations, without importing retained book liquidity. */
func (server *BookServer) flow(entry map[string]any, kind string) error {
	if kind != "snapshot" && kind != "update" {
		return errnie.Error(errnie.Err(errnie.Validation, "paper.book: unknown level3 frame type", nil))
	}
	for side, name := range []string{"bids", "asks"} {
		orders, found := entry[name].([]any)

		if !found {
			return errnie.Error(errnie.Err(errnie.Validation, "paper.book: missing level3 "+name, nil))
		}
		notional := decimal.NewFromInt64(0)
		for _, raw := range orders {
			order, found := raw.(map[string]any)

			if !found {
				return errnie.Error(errnie.Err(errnie.Validation, "paper.book: mutation is not an object", nil))
			}
			observed, err := server.notional(order, kind)

			if err != nil {
				return err
			}
			notional = notional.SetScale(max(notional.GetScale(), observed.GetScale())).Add(observed)
		}
		server.values[6+side], server.present[6+side] = notional.Float64(), true
		server.values[8+side], server.present[8+side] = float64(len(orders)), true
	}
	return nil
}

/* notional measures displayed add/modify size. A deletion only reports a removal. */
func (server *BookServer) notional(order map[string]any, kind string) (*decimal.Decimal, error) {
	event, _ := order["event"].(string)

	if event == "delete" {
		return decimal.NewFromInt64(0), nil
	}
	if event != "add" && event != "modify" && !(kind == "snapshot" && event == "") {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "paper.book: unknown level3 mutation", nil))
	}
	amounts := [2]*decimal.Decimal{}
	for index, name := range []string{"limit_price", "order_qty"} {
		var text string
		switch value := order[name].(type) {
		case json.Number:
			text = value.String()
		case string:
			text = value
		default:
			return nil, errnie.Error(errnie.Err(errnie.Validation, "paper.book: mutation requires "+name, nil))
		}
		value, err := core.ReadDecimal([]byte(text), name)

		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "paper.book: invalid "+name, err))
		}
		if value.Sign() < 0 {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "paper.book: negative "+name, nil))
		}
		amounts[index] = value
	}
	// Keep the product's complete decimal precision.
	scale := amounts[0].GetScale() + amounts[1].GetScale()
	return amounts[0].SetScale(scale).Mul(amounts[1]), nil
}

/* project exposes the SDK's reconciled touch and displayed depth as metric inputs. */
func (server *BookServer) project(held *book.Book) {
	for side, best := range []*book.Level{held.BestBid(), held.BestAsk()} {
		if best == nil {
			continue
		}
		server.values[side], server.present[side] = best.Price.Float64(), true
		server.values[2+side], server.present[2+side] = best.Quantity.Float64(), true
		quantity := decimal.NewFromInt64(0)
		for level := best; level != nil; {
			quantity = quantity.SetScale(max(quantity.GetScale(), level.Quantity.GetScale())).Add(level.Quantity)
			next := level.Lower

			if side == 1 {
				next = level.Higher
			}
			level = next
		}
		server.values[4+side], server.present[4+side] = quantity.Float64(), true
	}
}
