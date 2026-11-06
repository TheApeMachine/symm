package system

import (
	"time"

	"github.com/spf13/viper"
)

type WebSocket struct {
	Endpoints    *Endpoints
	PingInterval time.Duration
}

type Endpoints struct {
	Public      string
	Private     string
	Level3      string
	TradeVolume string
	Ticker      string
	Balance     string
}

func NewWebSocket() *WebSocket {
	viper.SetDefault("system.websocket.endpoints.public", "wss://ws.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.private", "wss://ws-auth.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.level3", "wss://ws-l3.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.balance", "/0/private/Balance")
	viper.SetDefault("system.websocket.endpoints.tradevolume", "/0/private/TradeVolume")
	viper.SetDefault("system.websocket.endpoints.ticker", "/0/public/Ticker")
	viper.SetDefault("system.websocket.ping_interval", 20*time.Second)

	return &WebSocket{
		Endpoints: &Endpoints{
			Public:      viper.GetString("system.websocket.endpoints.public"),
			Private:     viper.GetString("system.websocket.endpoints.private"),
			Level3:      viper.GetString("system.websocket.endpoints.level3"),
			TradeVolume: viper.GetString("system.websocket.endpoints.tradevolume"),
			Ticker:      viper.GetString("system.websocket.endpoints.ticker"),
			Balance:     viper.GetString("system.websocket.endpoints.balance"),
		},
		PingInterval: viper.GetDuration("system.websocket.ping_interval"),
	}
}
