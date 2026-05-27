package order

const MethodCancelOrder = "cancel_order"

/*
CancelRequest cancels one resting order by id.
*/
type CancelRequest struct {
	Method string       `json:"method"`
	Params CancelParams `json:"params"`
	ReqID  int          `json:"req_id,omitempty"`
}

/*
CancelParams holds cancel_order fields.
*/
type CancelParams struct {
	OrderID string `json:"order_id"`
	Token   string `json:"token"`
}

/*
CancelOrder builds a cancel_order frame.
*/
func CancelOrder(orderID, token string) CancelRequest {
	return CancelRequest{
		Method: MethodCancelOrder,
		Params: CancelParams{
			OrderID: orderID,
			Token:   token,
		},
	}
}

/*
LimitBuyBid posts a quote-notional limit buy at the inner bid.
*/
func LimitBuyBid(symbol string, notionalEUR, bidPrice float64, token string) Request {
	return Request{
		Method: MethodAddOrder,
		Params: AddParams{
			OrderType:    Limit,
			Side:         Buy,
			Symbol:       symbol,
			CashOrderQty: notionalEUR,
			LimitPrice:   bidPrice,
			Token:        token,
		},
	}
}
