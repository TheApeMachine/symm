package websocket

import (
	"encoding/json"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
)

const TradeVolumeEndpoint = kraken.TradeVolumeEndpoint

/*
WebSocket is the read/write surface for routed Kraken websocket frames.
*/
type WebSocket interface {
	Client() *spot.WebSocket
	On(channel string, action func([]byte))
	Write(params json.Marshaler) error
	Close()
}

/*
Rest is the REST dispatch surface for Kraken requests.
*/
type Rest interface {
	Get(path string, params json.Marshaler) ([]byte, error)
	Post(path string, params json.Marshaler) ([]byte, error)
}

/*
Conn is a Kraken transport that exposes both websocket and REST access.
*/
type Conn interface {
	WebSocket
	Rest
}
