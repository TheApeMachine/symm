package logic

/*
Side is the order side used by playbook actions.
*/
type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

/*
OrderType is the Kraken order type mapped from playbook actions.
*/
type OrderType string

const (
	OrderLimit             OrderType = "limit"
	OrderMarket            OrderType = "market"
	OrderSettlePosition    OrderType = "settle-position"
	OrderIceberg           OrderType = "iceberg"
	OrderStopLoss          OrderType = "stop-loss"
	OrderStopLossLimit     OrderType = "stop-loss-limit"
	OrderTakeProfit        OrderType = "take-profit"
	OrderTakeProfitLimit   OrderType = "take-profit-limit"
	OrderTrailingStop      OrderType = "trailing-stop"
	OrderTrailingStopLimit OrderType = "trailing-stop-limit"
)
