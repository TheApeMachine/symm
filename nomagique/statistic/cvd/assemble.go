package cvd

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Fill is one aggressive execution.
*/
type Fill struct {
	Symbol string
	Side   string
	Price  float64
	Qty    float64
	At     int64
}

/*
Assemble gathers trade inputs into one Fill.
No structs, pure Value closure.
*/
type Assemble types.Value[map[string]any, *Fill]

func NewAssemble() Assemble {
	return func(input map[string]any) *Fill {
		if input == nil {
			return nil
		}

		trade, ok := input["trade"].(map[string]any)
		if !ok {
			trade = input
		}

		data, ok := trade["data"].(map[string]any)
		if !ok {
			data = trade
		}

		side, _ := data["side"].(string)
		if side != "buy" && side != "sell" {
			return nil
		}

		symbol, _ := data["symbol"].(string)
		var price float64
		switch p := data["price"].(type) {
		case float64:
			price = p
		case int:
			price = float64(p)
		case int64:
			price = float64(p)
		}

		var qty float64
		switch q := data["qty"].(type) {
		case float64:
			qty = q
		case int:
			qty = float64(q)
		case int64:
			qty = float64(q)
		}

		var at int64
		switch ts := data["timestamp"].(type) {
		case int64:
			at = ts
		case float64:
			at = int64(ts)
		case int:
			at = int64(ts)
		}

		return &Fill{
			Symbol: symbol,
			Side:   side,
			Price:  price,
			Qty:    qty,
			At:     at,
		}
	}
}
