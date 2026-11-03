package ui

import "github.com/theapemachine/symm/types"

/*
Focus writes one dashboard frame {"channel":{symbol: reading}} when the
client is attached and focused on symbol. Non-focus metrics stay off the
wire so the hub queue is not flooded by the full universe each tick.
*/
func Focus(channel, symbol string, reading any) {
	if reading == nil || channel == "" || !types.Allows(symbol) {
		return
	}

	if UI == nil || !UI.Active() {
		return
	}

	UI.WriteJSON(map[string]any{
		channel: map[string]any{symbol: reading},
	})
}
