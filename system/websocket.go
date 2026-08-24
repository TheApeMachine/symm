package system

import "github.com/spf13/viper"

type WebSocket struct {
	Endpoints Endpoints
}

type Endpoints struct {
	Public  string
	Private string
	Level3  string
}

func NewWebSocket() *WebSocket {
	viper.SetDefault("system.websocket.endpoints.public", "wss://ws.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.private", "wss://ws-auth.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.level3", "wss://ws-l3.kraken.com/v2")

	return &WebSocket{
		Endpoints: Endpoints{
			Public:  viper.GetString("system.websocket.endpoints.public"),
			Private: viper.GetString("system.websocket.endpoints.private"),
			Level3:  viper.GetString("system.websocket.endpoints.level3"),
		},
	}
}
