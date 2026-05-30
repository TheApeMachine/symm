package public

import "encoding/json"

const MethodSubscribe = "subscribe"

type SocketMessage struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
}

/*
Subscription is the Kraken WebSocket v2 control frame: a method ("subscribe") and
the channel-specific params payload sent to open a feed.
*/
type Subscription struct {
	Method string `json:"method"`
	Params any    `json:"params"`
}
