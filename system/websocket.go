package system

import (
	"time"

	"github.com/spf13/viper"
)

type WebSocket struct {
	Endpoints *Endpoints

	// PingInterval is how often a session sends its keepalive. A half-open
	// connection — one side gone without a close, as when a mobile link drops
	// out of reception — is invisible to a socket that only reads: the venue
	// waits for data that will never come and nothing errors. The periodic
	// write is what exposes it, since the unacknowledged retransmits eventually
	// fail the socket and surface the disconnect.
	PingInterval time.Duration
}

type Endpoints struct {
	Public  string
	Private string
	Level3  string
	Futures string
}

func NewWebSocket() *WebSocket {
	viper.SetDefault("system.websocket.endpoints.public", "wss://ws.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.private", "wss://ws-auth.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.level3", "wss://ws-l3.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.futures", "wss://futures.kraken.com/ws/v1")
	viper.SetDefault("system.websocket.ping_interval", 20*time.Second)

	return &WebSocket{
		Endpoints: &Endpoints{
			Public:  viper.GetString("system.websocket.endpoints.public"),
			Private: viper.GetString("system.websocket.endpoints.private"),
			Level3:  viper.GetString("system.websocket.endpoints.level3"),
			Futures: viper.GetString("system.websocket.endpoints.futures"),
		},
		PingInterval: viper.GetDuration("system.websocket.ping_interval"),
	}
}
