package order

/*
AmendRequest updates trigger price on a resting stop order.
*/
type AmendRequest struct {
	Method string      `json:"method"`
	Params AmendParams `json:"params"`
	ReqID  int         `json:"req_id,omitempty"`
}

/*
AmendParams holds amend_order fields for stop trigger updates.
*/
type AmendParams struct {
	OrderID      string  `json:"order_id"`
	TriggerPrice float64 `json:"trigger_price"`
	Token        string  `json:"token"`
}

/*
AmendStopTrigger builds an amend_order frame for a ratcheted stop.
*/
func AmendStopTrigger(orderID string, triggerPrice float64, token string) AmendRequest {
	return AmendRequest{
		Method: MethodAmendOrder,
		Params: AmendParams{
			OrderID:      orderID,
			TriggerPrice: triggerPrice,
			Token:        token,
		},
	}
}
