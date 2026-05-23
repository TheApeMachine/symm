package order

/*
OrderType is the type of order to add to Kraken.

The execution model of the order.

market              : The full order quantity executes immediately at the best available price in the order book.
limit               : The full order quantity is placed immediately with a limit price restriction to only trade at this price or better.
stop-loss           : A market order is triggered when the reference price reaches the stop price (from an unfavourable direction).
stop-loss-limit     : A limit order is triggered when the reference price reaches the stop price (from an unfavourable direction).
take-profit         : A market order is triggered when the reference price reaches the stop price (from an favourable direction).
take-profit-limit   : A limit order is triggered when the reference price reaches the stop price (from an favourable direction).
trailing-stop       : A market order is triggered when the market reverts a specified distance from the peak price.
trailing-stop-limit : A limit order is triggered when the market reverts a specified distance from the peak price.
iceberg             : Hides the full order size by only showing your chosen display size in the book at your limit price.
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

type Side string

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

/*
AddOrder is the struct for adding an order to Kraken.
*/
type AddOrder struct {
	Method string      `json:"method"`
	Params OrderParams `json:"params"`
}

type OrderParams struct {
	OrderType OrderType     `json:"order_type"`
	Side      Side          `json:"side"`
	OrderQty  int           `json:"order_qty"`
	Symbol    string        `json:"symbol"`
	Triggers  OrderTriggers `json:"triggers"`
	Token     string        `json:"token"`
}

type OrderTriggers struct {
	Reference string  `json:"reference"`
	Price     float64 `json:"price"`
	PriceType string  `json:"price_type"`
}

func NewAddOrder(
	orderType OrderType,
	side Side,
	symbol string,
	orderQty int,
	triggers OrderTriggers,
	token string,
) *AddOrder {
	return &AddOrder{
		Method: "AddOrder",
		Params: OrderParams{
			OrderType: orderType,
			Side:      side,
			OrderQty:  orderQty,
			Symbol:    symbol,
			Triggers:  triggers,
			Token:     token,
		},
	}
}
