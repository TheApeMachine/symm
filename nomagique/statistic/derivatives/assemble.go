package derivatives

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Fill represents an aggressive execution or liquidation on derivatives.
*/
type Fill struct {
	Symbol string
	Side   string
	Price  float64
	Qty    float64
	Kind   string
	At     int64
}

/*
Snapshot represents market mark, last, and open interest for a derivative instrument.
*/
type Snapshot struct {
	Symbol string
	Index  float64
	Last   float64
	Mark   float64
	OI     float64
	At     int64
}

/*
TradeAssemble gathers futures/derivatives trade input into one Fill.
No structs, pure Value closure.
*/
type TradeAssemble types.Value[map[string]any, *Fill]

func NewTradeAssemble() TradeAssemble {
	return func(input map[string]any) *Fill {
		if input == nil {
			return nil
		}

		trade, ok := input["futures"].(map[string]any)
		if !ok {
			trade, ok = input["trade"].(map[string]any)
			if !ok {
				trade = input
			}
		}

		data, ok := trade["data"].(map[string]any)
		if !ok {
			data = trade
		}

		symbol, _ := data["symbol"].(string)
		side, _ := data["side"].(string)
		kind, _ := data["type"].(string)

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
		switch t := data["timestamp"].(type) {
		case int64:
			at = t
		case int:
			at = int64(t)
		case float64:
			at = int64(t)
		}

		if price <= 0 || qty <= 0 {
			return nil
		}

		return &Fill{
			Symbol: symbol,
			Side:   side,
			Price:  price,
			Qty:    qty,
			Kind:   kind,
			At:     at,
		}
	}
}

/*
Assemble gathers derivatives ticker/mark/OI inputs into one Snapshot.
No structs, pure Value closure.
*/
type Assemble types.Value[map[string]any, *Snapshot]

func NewAssemble() Assemble {
	return func(input map[string]any) *Snapshot {
		if input == nil {
			return nil
		}

		ticker, ok := input["ticker"].(map[string]any)
		if !ok {
			ticker = input
		}

		data, ok := ticker["data"].(map[string]any)
		if !ok {
			data = ticker
		}

		symbol, _ := data["symbol"].(string)

		var last float64
		switch l := data["last"].(type) {
		case float64:
			last = l
		case int:
			last = float64(l)
		case int64:
			last = float64(l)
		}

		var index float64
		switch idx := data["index"].(type) {
		case float64:
			index = idx
		case int:
			index = float64(idx)
		case int64:
			index = float64(idx)
		}
		if index <= 0 {
			index = last
		}

		var mark float64
		switch m := data["mark"].(type) {
		case float64:
			mark = m
		case int:
			mark = float64(m)
		case int64:
			mark = float64(m)
		}
		if mark <= 0 {
			mark = last
		}

		var oi float64
		switch o := data["openInterest"].(type) {
		case float64:
			oi = o
		case int:
			oi = float64(o)
		case int64:
			oi = float64(o)
		}

		var at int64
		switch t := data["timestamp"].(type) {
		case int64:
			at = t
		case int:
			at = int64(t)
		case float64:
			at = int64(t)
		}

		if last <= 0 {
			return nil
		}

		return &Snapshot{
			Symbol: symbol,
			Index:  index,
			Last:   last,
			Mark:   mark,
			OI:     oi,
			At:     at,
		}
	}
}
