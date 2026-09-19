package hawkes

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewAssemble creates a Value closure that gathers trade inputs into one Hawkes Event.
State and behavior are expressed directly as a Value closure with no struct overhead.
*/
type Assemble types.Value[map[string]any, Event]
func NewAssemble() Assemble {
	return func(input map[string]any) Event {
		if input == nil {
			return Event{}
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
			return Event{}
		}

		symbol, _ := data["symbol"].(string)
		var at int64
		switch ts := data["timestamp"].(type) {
		case int64:
			at = ts
		case float64:
			at = int64(ts)
		}

		return Event{
			Symbol: symbol,
			Side:   side,
			At:     at,
		}
	}
}
