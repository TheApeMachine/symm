package core

const KRAKEN_API_URL = "https://api.kraken.com"
const KRAKEN_BASE_URL = "https://api.kraken.com/0/public"
const KRAKEN_WS_URL = "wss://ws.kraken.com/v2"
const KRAKEN_WS_AUTH_URL = "wss://ws-auth.kraken.com/v2"

const (
	WebSocketToken = "/0/private/GetWebSocketsToken"
)

const (
	ChannelInstrument = "instrument"
	ChannelTicker     = "ticker"
	ChannelBook       = "book"
	ChannelTrades     = "trades"
	ChannelExecutions = "executions"
	ChannelBalances   = "balances"
)
