package replay

import (
	"encoding/json"
	"time"
)

const (
	TransportMeta = "meta"
	TransportWS   = "ws"
	TransportREST = "rest"
)

const (
	DirectionIn  = "in"
	DirectionOut = "out"
)

/*
Line is one JSONL record: an exact Kraken WebSocket frame, REST exchange, or
session metadata written by Recorder and consumed by Hub on playback.
*/
type Line struct {
	Timestamp time.Time       `json:"ts"`
	Transport string          `json:"transport"`
	Channel   string          `json:"channel"`
	Direction string          `json:"direction,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}
