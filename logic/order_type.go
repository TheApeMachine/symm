package logic

type OrderType string

const (
	OrderLimit             OrderType = "limit"
	OrderMarket            OrderType = "market"
	OrderIceberg           OrderType = "iceberg"
	OrderStopLoss          OrderType = "stop-loss"
	OrderStopLossLimit     OrderType = "stop-loss-limit"
	OrderTakeProfit        OrderType = "take-profit"
	OrderTakeProfitLimit   OrderType = "take-profit-limit"
	OrderTrailingStop      OrderType = "trailing-stop"
	OrderTrailingStopLimit OrderType = "trailing-stop-limit"
	OrderSettlePosition    OrderType = "settle-position"
)
