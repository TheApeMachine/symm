package order

const (
	MethodAddOrder   = "add_order"
	MethodAmendOrder = "amend_order"
)

/*
OrderType is the Kraken WebSocket v2 order execution model.
*/
type OrderType string

const (
	Limit             OrderType = "limit"
	Market            OrderType = "market"
	Iceberg           OrderType = "iceberg"
	StopLoss          OrderType = "stop-loss"
	StopLossLimit     OrderType = "stop-loss-limit"
	TakeProfit        OrderType = "take-profit"
	TakeProfitLimit   OrderType = "take-profit-limit"
	TrailingStop      OrderType = "trailing-stop"
	TrailingStopLimit OrderType = "trailing-stop-limit"
)

/*
Side is the order book side.
*/
type Side string

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)
