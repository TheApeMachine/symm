package types



/*
Channel names on the system workspace bus. One Workspace is shared by the
whole pipeline; stages subscribe to the named channels they consume and
publish to the named channels they produce.
*/
const (
	ChannelTickers        = "tickers"
	ChannelTrades         = "trades"
	ChannelLevel3         = "level3"
	ChannelFuturesTickers = "futures_tickers"
	ChannelFuturesTrades  = "futures_trades"

	ChannelSignals      = "signals"
	ChannelPerspectives = "perspectives"
	ChannelDecisions    = "decisions"
	ChannelExecutions   = "executions"
	ChannelOrders       = "orders"
	ChannelPositions    = "positions"
	ChannelPnl          = "pnl"
	ChannelTelemetry    = "telemetry"
)

