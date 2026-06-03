package trading

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

const (
	MethodAddOrder             = "add_order"
	MethodAmendOrder           = "amend_order"
	MethodCancelOrder          = "cancel_order"
	MethodCancelAll            = "cancel_all"
	MethodCancelAllOrdersAfter = "cancel_all_orders_after"
	MethodBatchAdd             = "batch_add"
	MethodBatchCancel          = "batch_cancel"
	MethodEditOrder            = "edit_order"
)

/*
Side and OrderType mirror Kraken WebSocket v2 add_order params.
See https://docs.kraken.com/api/docs/websocket-v2/add_order/
*/
type OrderType string

const (
	Limit             OrderType = "limit"
	Market            OrderType = "market"
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
Triggers holds stop/take-profit trigger params for triggered order types.
*/
type Triggers struct {
	Reference string  `json:"reference,omitempty"`
	Price     float64 `json:"price"`
	PriceType string  `json:"price_type,omitempty"`
}

/*
AddParams is the params object on an add_order frame.
*/
type AddParams struct {
	OrderType  OrderType `json:"order_type"`
	Side       Side      `json:"side"`
	Symbol     string    `json:"symbol"`
	OrderQty   float64   `json:"order_qty,omitempty"`
	LimitPrice float64   `json:"limit_price,omitempty"`
	ClOrdID    string    `json:"cl_ord_id,omitempty"`
	PostOnly   bool      `json:"post_only,omitempty"`
	Triggers   *Triggers `json:"triggers,omitempty"`
	Token      string    `json:"token,omitempty"`
}

type AmendParams struct {
	OrderID    string  `json:"order_id,omitempty"`
	ClOrdID    string  `json:"cl_ord_id,omitempty"`
	OrderQty   float64 `json:"order_qty,omitempty"`
	LimitPrice float64 `json:"limit_price,omitempty"`
	Symbol     string  `json:"symbol,omitempty"`
	Token      string  `json:"token,omitempty"`
}

type CancelParams struct {
	OrderID      []string `json:"order_id,omitempty"`
	ClOrdID      []string `json:"cl_ord_id,omitempty"`
	OrderUserref []int    `json:"order_userref,omitempty"`
	Token        string   `json:"token,omitempty"`
}

type CancelAllParams struct {
	Token string `json:"token,omitempty"`
}

type CancelAllOrdersAfterParams struct {
	Timeout int    `json:"timeout"`
	Token   string `json:"token,omitempty"`
}

type BatchAddParams struct {
	Symbol   string      `json:"symbol"`
	Orders   []AddParams `json:"orders"`
	Token    string      `json:"token,omitempty"`
	Validate bool        `json:"validate,omitempty"`
}

type BatchCancelParams struct {
	Orders  []string `json:"orders,omitempty"`
	ClOrdID []string `json:"cl_ord_id,omitempty"`
	Token   string   `json:"token,omitempty"`
}

type EditParams struct {
	OrderID    string  `json:"order_id"`
	Symbol     string  `json:"symbol"`
	OrderQty   float64 `json:"order_qty,omitempty"`
	LimitPrice float64 `json:"limit_price,omitempty"`
	Token      string  `json:"token,omitempty"`
}

/*
Ack is the add_order method response envelope.
*/
type Ack struct {
	Method  string `json:"method"`
	Success bool   `json:"success"`
	Error   string `json:"error"`
	ReqID   int    `json:"req_id"`
	Result  struct {
		OrderID string `json:"order_id"`
		ClOrdID string `json:"cl_ord_id"`
	} `json:"result"`
}

type OrderUpdate struct {
	OrderID      string `json:"order_id"`
	OrderUserref int    `json:"order_userref"`
}

type OrderClient struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
}

func NewOrderClient(ctx context.Context, pool *qpool.Q) *OrderClient {
	ctx, cancel := context.WithCancel(ctx)

	client := &OrderClient{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
	}

	for _, channel := range []string{"kraken:private"} {
		client.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		client.subscribers[channel] = client.broadcasts[channel].Subscribe(channel, 1024)
	}

	return client
}

func (client *OrderClient) AddOrder(params AddParams) error {
	if params.ClOrdID == "" {
		return errnie.Error(errnie.Require(map[string]any{
			"params.ClOrdID": params.ClOrdID,
		}))
	}

	client.broadcasts["kraken:private"].Send(&qpool.QValue[any]{
		Type: "kraken:private",
		Value: map[string]any{
			"method": MethodAddOrder,
			"params": params,
		},
	})

	return nil
}

func (client *OrderClient) AmendOrder(params AmendParams) error {
	client.broadcasts["kraken:private"].Send(&qpool.QValue[any]{
		Type: "kraken:private",
		Value: map[string]any{
			"method": MethodAmendOrder,
			"params": params,
		},
	})

	return nil
}

func (client *OrderClient) CancelOrder(params CancelParams) error {
	client.broadcasts["kraken:private"].Send(&qpool.QValue[any]{
		Type: "kraken:private",
		Value: map[string]any{
			"method": MethodCancelOrder,
			"params": params,
		},
	})

	return nil
}

func (client *OrderClient) CancelAll(params CancelAllParams) error {
	client.broadcasts["kraken:private"].Send(&qpool.QValue[any]{
		Type: "kraken:private",
		Value: map[string]any{
			"method": MethodCancelAll,
			"params": params,
		},
	})

	return nil
}

func (client *OrderClient) CancelAllOrdersAfter(params CancelAllOrdersAfterParams) error {
	client.broadcasts["kraken:private"].Send(&qpool.QValue[any]{
		Type: "kraken:private",
		Value: map[string]any{
			"method": MethodCancelAllOrdersAfter,
			"params": params,
		},
	})

	return nil
}

func (client *OrderClient) BatchAdd(params BatchAddParams) error {
	client.broadcasts["kraken:private"].Send(&qpool.QValue[any]{
		Type: "kraken:private",
		Value: map[string]any{
			"method": MethodBatchAdd,
			"params": params,
		},
	})

	return nil
}

func (client *OrderClient) BatchCancel(params BatchCancelParams) error {
	client.broadcasts["kraken:private"].Send(&qpool.QValue[any]{
		Type: "kraken:private",
		Value: map[string]any{
			"method": MethodBatchCancel,
			"params": params,
		},
	})

	return nil
}

func (client *OrderClient) EditOrder(params EditParams) error {
	client.broadcasts["kraken:private"].Send(&qpool.QValue[any]{
		Type: "kraken:private",
		Value: map[string]any{
			"method": MethodEditOrder,
			"params": params,
		},
	})

	return nil
}

func (client *OrderClient) Close() error {
	client.cancel()
	return nil
}
