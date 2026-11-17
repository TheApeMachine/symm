package system

import (
	"time"

	"github.com/spf13/viper"
)

type WebSocket struct {
	Endpoints         *Endpoints
	PingInterval      time.Duration
	PrivateRefresh    time.Duration
	PrivateBackoffMax time.Duration
	TickerRefresh     time.Duration
	TickerBackoffMax  time.Duration
	TickerBatchSize   int
}

type Endpoints struct {
	Public       string
	Private      string
	Level3       string
	TradeVolume  string
	Ticker       string
	Balance       string
	TradeBalance  string
	TradesHistory string
	QueryOrders   string
}

func NewWebSocket() *WebSocket {
	viper.SetDefault("system.websocket.endpoints.public", "wss://ws.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.private", "wss://ws-auth.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.level3", "wss://ws-l3.kraken.com/v2")
	viper.SetDefault("system.websocket.endpoints.balance", "https://api.kraken.com/0/private/Balance")
	viper.SetDefault("system.websocket.endpoints.trade_balance", "https://api.kraken.com/0/private/TradeBalance")
	viper.SetDefault("system.websocket.endpoints.trades_history", "https://api.kraken.com/0/private/TradesHistory")
	viper.SetDefault("system.websocket.endpoints.query_orders", "https://api.kraken.com/0/private/QueryOrders")
	viper.SetDefault("system.websocket.endpoints.tradevolume", "https://api.kraken.com/0/private/TradeVolume")
	viper.SetDefault("system.websocket.endpoints.ticker", "https://api.kraken.com/0/public/Ticker")
	viper.SetDefault("system.websocket.ping_interval", 20*time.Second)
	// Balance + TradeBalance are +1 REST counter each. Intermediate decays
	// 0.5/s (Pro 1/s), so a 5s private poll of both (+2/5s) stays under the
	// Intermediate/Pro caps; Starter (0.33/s) is tighter if other private
	// calls share the key. See https://docs.kraken.com/exchange/guides/rest/ratelimits
	viper.SetDefault("system.websocket.private_refresh", 5*time.Second)
	// Transport failures on private REST grow backoff up to this cap so a
	// dead path does not keep spending the call counter.
	viper.SetDefault("system.websocket.private_backoff_max", 5*time.Minute)
	// Unfiltered / multi-pair public Ticker often returns EService:Unavailable;
	// refresh a round-robin slice and back off on hard failures.
	viper.SetDefault("system.websocket.ticker_refresh", 30*time.Second)
	viper.SetDefault("system.websocket.ticker_backoff_max", 15*time.Minute)
	viper.SetDefault("system.websocket.ticker_batch_size", 128)

	return &WebSocket{
		Endpoints: &Endpoints{
			Public:       viper.GetString("system.websocket.endpoints.public"),
			Private:      viper.GetString("system.websocket.endpoints.private"),
			Level3:       viper.GetString("system.websocket.endpoints.level3"),
			TradeVolume:  viper.GetString("system.websocket.endpoints.tradevolume"),
			Ticker:       viper.GetString("system.websocket.endpoints.ticker"),
			Balance:      viper.GetString("system.websocket.endpoints.balance"),
			TradeBalance:  viper.GetString("system.websocket.endpoints.trade_balance"),
			TradesHistory: viper.GetString("system.websocket.endpoints.trades_history"),
			QueryOrders:   viper.GetString("system.websocket.endpoints.query_orders"),
		},
		PingInterval:      viper.GetDuration("system.websocket.ping_interval"),
		PrivateRefresh:    viper.GetDuration("system.websocket.private_refresh"),
		PrivateBackoffMax: viper.GetDuration("system.websocket.private_backoff_max"),
		TickerRefresh:     viper.GetDuration("system.websocket.ticker_refresh"),
		TickerBackoffMax:  viper.GetDuration("system.websocket.ticker_backoff_max"),
		TickerBatchSize:   viper.GetInt("system.websocket.ticker_batch_size"),
	}
}
