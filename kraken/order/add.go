package order

/*
Request is a Kraken WebSocket v2 trading request frame.
*/
type Request struct {
	Method string    `json:"method"`
	Params AddParams `json:"params"`
	ReqID  int       `json:"req_id,omitempty"`
}

/*
AddParams holds add_order fields for spot trading. ClOrdID is the client
order id used for reconciliation on ack timeout or reconnect — Kraken echoes
it back in the Ack and in subsequent executions frames.
*/
type AddParams struct {
	OrderType    OrderType        `json:"order_type"`
	Side         Side             `json:"side"`
	Symbol       string           `json:"symbol"`
	OrderQty     float64          `json:"order_qty,omitempty"`
	CashOrderQty float64          `json:"cash_order_qty,omitempty"`
	LimitPrice   float64          `json:"limit_price,omitempty"`
	Token        string           `json:"token"`
	ClOrdID      string           `json:"cl_ord_id,omitempty"`
	Conditional  *ConditionalStop `json:"conditional,omitempty"`
}

/*
ConditionalStop attaches a secondary stop-loss-limit on primary fill (OTO).
*/
type ConditionalStop struct {
	OrderType    OrderType `json:"order_type"`
	TriggerPrice float64   `json:"trigger_price"`
	LimitPrice   float64   `json:"limit_price,omitempty"`
}

/*
MarketBuyCash builds a quote-notional market buy for EUR-denominated pairs.
*/
func MarketBuyCash(
	symbol string,
	notionalEUR float64,
	stopPrice float64,
	limitBelowStop float64,
	token string,
) Request {
	params := AddParams{
		OrderType:    Market,
		Side:         Buy,
		Symbol:       symbol,
		CashOrderQty: notionalEUR,
		Token:        token,
	}

	if stopPrice > 0 {
		params.Conditional = &ConditionalStop{
			OrderType:    StopLossLimit,
			TriggerPrice: stopPrice,
			LimitPrice:   limitBelowStop,
		}
	}

	return Request{
		Method: MethodAddOrder,
		Params: params,
	}
}

/*
MarketSellBase builds a base-quantity market sell to close a long.
*/
func MarketSellBase(symbol string, baseQty float64, token string) Request {
	return Request{
		Method: MethodAddOrder,
		Params: AddParams{
			OrderType: Market,
			Side:      Sell,
			Symbol:    symbol,
			OrderQty:  baseQty,
			Token:     token,
		},
	}
}
