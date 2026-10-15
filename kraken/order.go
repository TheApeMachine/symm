package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
)

/*
OrderResponseResult identifies the venue order created by Kraken.
*/
type OrderResponseResult struct {
	OrderID      string `json:"order_id"`
	OrderUserref int    `json:"order_userref"`
}

/*
OrderResponse is Kraken's acknowledgement of an order request.
*/
type OrderResponse struct {
	Method  string              `json:"method"`
	Result  OrderResponseResult `json:"result"`
	Error   string              `json:"error"`
	Success bool                `json:"success"`
	ReqID   int64               `json:"req_id"`
	TimeIn  time.Time           `json:"time_in"`
	TimeOut time.Time           `json:"time_out"`
}

/*
MarshalJSON encodes an order acknowledgement for paper and test transports.
*/
func (order *OrderResponse) MarshalJSON() ([]byte, error) {
	type alias OrderResponse
	return sonic.Marshal((*alias)(order))
}

/*
Action returns the transport method used to route this acknowledgement.
*/
func (order *OrderResponse) Action() string {
	return order.Method
}

/*
IsSuccess reports whether Kraken accepted the order request.
*/
func (order *OrderResponse) IsSuccess() bool {
	return order.Success
}

/*
NewOrderResponseFromMap adapts a paper fill acknowledgement to Kraken's model.
*/
func NewOrderResponseFromMap(model datura.Map[any], reqID int64) *OrderResponse {
	orderID, _ := model["order_id"].(string)
	success := false

	if s, ok := model["success"].(bool); ok {
		success = s
	}

	if !success && orderID != "" {
		success = true
	}

	return &OrderResponse{
		Method: "add_order",
		Result: OrderResponseResult{
			OrderID: orderID,
		},
		Success: success,
		ReqID:   reqID,
	}
}

type AddOrderRequest struct {
	Token   string `json:"token"`
	Pair    string `json:"symbol"`
	Type    string `json:"side"`
	OrdType string `json:"order_type"`
	// Volume is the venue size string. WS v2 names it order_qty (REST still
	// says volume); paper and live add_order both consume order_qty.
	Volume  string `json:"order_qty"`
	Price   string `json:"limit_price,omitempty"`
	ClOrdId string `json:"cl_ord_id,omitempty"`
}

type AddOrderMessage struct {
	Method string          `json:"method"`
	Params AddOrderRequest `json:"params"`
}

func NewAddOrderMessage(token string, req *AddOrderRequest) AddOrderMessage {
	req.Token = token
	if req.OrdType == "" {
		req.OrdType = "market"
		if req.Price != "" {
			req.OrdType = "limit"
		}
	}
	return AddOrderMessage{
		Method: "add_order",
		Params: *req,
	}
}

func (msg AddOrderMessage) MarshalJSON() ([]byte, error) {
	type alias AddOrderMessage
	return sonic.Marshal((*alias)(&msg))
}

/*
ToMeasurement converts the AddOrderRequest into a structured Measurement
without any serialized raw shortcuts.
*/
func (req *AddOrderRequest) ToMeasurement(epochs ...int64) *data.Measurement {
	if req == nil {
		return nil
	}

	epoch := system.Epoch.Load()

	if len(epochs) > 0 && epochs[0] > 0 {
		epoch = epochs[0]
	}

	now := time.Now().UTC()

	if epoch <= 0 {
		epoch = now.UnixNano()
	}

	symbol := req.Pair
	orderType := req.OrdType

	if orderType == "" {
		if req.Price != "" {
			orderType = "limit"
		}

		if orderType == "" {
			orderType = "market"
		}
	}

	meas := data.NewMeasurement(
		epoch,
		symbol,
		"orders",
		now,
		now,
		&data.StringEntry{
			Key:   "symbol",
			Value: symbol,
		},
		&data.StringEntry{
			Key:   "cl_ord_id",
			Value: req.ClOrdId,
		},
		&data.StringEntry{
			Key:   "order_type",
			Value: orderType,
		},
		&data.StringEntry{
			Key:   "type",
			Value: req.Type,
		},
		&data.StringEntry{
			Key:   "side",
			Value: req.Type,
		},
	)

	var metrics []*data.Metric

	if req.Volume != "" {
		if vol, err := decimal.NewFromString(req.Volume); err == nil && vol != nil {
			metrics = append(metrics, data.NewExactMetric("volume", vol, data.UnitQuantity, data.TimescaleInstantaneous))
		}
	}

	if req.Price != "" {
		if limitPrice, err := decimal.NewFromString(req.Price); err == nil && limitPrice != nil {
			metrics = append(metrics, data.NewExactMetric(
				"limit_price", limitPrice, data.UnitPrice, data.TimescaleInstantaneous,
			))
		}
	}

	if len(metrics) > 0 {
		meas.Write(metrics...)
	}

	meas.Finalize()
	return meas
}

/*
NewAddOrderRequestFromMeasurement reconstructs an AddOrderRequest from a Measurement.
*/
func NewAddOrderRequestFromMeasurement(meas *data.Measurement) (*AddOrderRequest, error) {
	if meas == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[kraken.order] cannot convert nil measurement to order request", nil,
		))
	}

	meta := make(map[string]string)

	for entry := range meas.Meta() {
		if entry != nil && entry.Key != "" {
			meta[entry.Key] = entry.Value
		}
	}

	symbol := meas.Label

	if s, ok := meta["symbol"]; ok && s != "" {
		symbol = s
	}

	req := &AddOrderRequest{
		Pair:    symbol,
		Type:    meta["side"],
		OrdType: meta["order_type"],
		ClOrdId: meta["cl_ord_id"],
	}

	if req.Type == "" {
		req.Type = meta["type"]
	}

	if entry := data.Pull(meas.Read("volume")); entry != nil && entry.Metric != nil && entry.Metric.Exact() != nil {
		req.Volume = entry.Metric.Exact().String()
	}

	if entry := data.Pull(meas.Read("limit_price")); entry != nil && entry.Metric != nil && entry.Metric.Exact() != nil {
		req.Price = entry.Metric.Exact().String()
	}

	return req, nil
}
