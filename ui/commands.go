package ui

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/qpool"
)

/*
SubscriptionCommands forwards dashboard watch commands to the shared subscriptions group.
*/
type SubscriptionCommands struct {
	subscriptions *qpool.BroadcastGroup
}

/*
NewSubscriptionCommands wires websocket subscribe/unsubscribe to PublicClient.
*/
func NewSubscriptionCommands(pool *qpool.Q) *SubscriptionCommands {
	return &SubscriptionCommands{
		subscriptions: pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond),
	}
}

func (commands *SubscriptionCommands) HandleCommand(raw any) {
	payload, ok := raw.([]byte)

	if !ok || len(payload) == 0 {
		return
	}

	var command map[string]any

	if json.Unmarshal(payload, &command) != nil {
		return
	}

	op, _ := command["op"].(string)

	switch op {
	case "subscribe", "unsubscribe":
		symbols := parseSymbolList(command["symbols"])

		if len(symbols) == 0 {
			return
		}

		commands.subscriptions.Send(&qpool.QValue[any]{Value: symbols})
	default:
		return
	}
}

func parseSymbolList(raw any) []string {
	rawSymbols, ok := raw.([]any)

	if !ok || len(rawSymbols) == 0 {
		return nil
	}

	symbols := make([]string, 0, len(rawSymbols))

	for _, rawSymbol := range rawSymbols {
		symbol, ok := rawSymbol.(string)

		if !ok || symbol == "" {
			continue
		}

		symbols = append(symbols, symbol)
	}

	if len(symbols) == 0 {
		return nil
	}

	return symbols
}
