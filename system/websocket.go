package system

import (
	"time"

	"github.com/spf13/viper"
)

type WebSocket struct {
	Endpoints      *Endpoints
	PingInterval   time.Duration
	DecayRate      float64
	CallsPerUpdate int
	CallsPerSecond int
}

type Endpoints struct {
	Public       string
	Private      string
	Level3       string
	Futures      string
	TradeBalance string
	TradeVolume  string
}

func NewWebSocket() *WebSocket {
	viper.SetDefault("system.websocket.endpoints.public", "wss://ws.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.private", "wss://ws-auth.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.level3", "wss://ws-l3.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.futures", "wss://futures.kraken.com/ws/v1")
	viper.SetDefault("system.websocket.endpoints.tradebalance", "/0/private/TradeBalance")
	viper.SetDefault("system.websocket.endpoints.tradevolume", "/0/private/TradeVolume")
	viper.SetDefault("system.websocket.ping_interval", 20*time.Second)
	viper.SetDefault("system.websocket.decay_rate", 0.33)
	viper.SetDefault("system.websocket.calls_per_update", 2)
	viper.SetDefault("system.websocket.calls_per_second", 1)

	return &WebSocket{
		Endpoints: &Endpoints{
			Public:       viper.GetString("system.websocket.endpoints.public"),
			Private:      viper.GetString("system.websocket.endpoints.private"),
			Level3:       viper.GetString("system.websocket.endpoints.level3"),
			Futures:      viper.GetString("system.websocket.endpoints.futures"),
			TradeBalance: viper.GetString("system.websocket.endpoints.tradebalance"),
			TradeVolume:  viper.GetString("system.websocket.endpoints.tradevolume"),
		},
		PingInterval:   viper.GetDuration("system.websocket.ping_interval"),
		DecayRate:      viper.GetFloat64("system.websocket.decay_rate"),
		CallsPerUpdate: viper.GetInt("system.websocket.calls_per_update"),
		CallsPerSecond: viper.GetInt("system.websocket.calls_per_second"),
	}
}
