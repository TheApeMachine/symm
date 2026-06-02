package trading

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
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
	Limit           OrderType = "limit"
	Market          OrderType = "market"
	StopLoss        OrderType = "stop-loss"
	StopLossLimit   OrderType = "stop-loss-limit"
	TakeProfit      OrderType = "take-profit"
	TakeProfitLimit OrderType = "take-profit-limit"
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

/*
Client sends order frames to kraken:private and tracks acknowledgements.
*/
type Client struct {
	ctx    context.Context
	cancel context.CancelFunc
	orders *qpool.BroadcastGroup
	ledger *Ledger
}

func NewOrder(ctx context.Context, pool *qpool.Q) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)

	orderClient := &Client{
		ctx:    ctx,
		cancel: cancel,
		orders: pool.CreateBroadcastGroup("kraken:private", 10*time.Millisecond),
	}

	ledger, err := NewLedger(ctx, pool, orderClient.orders)

	if err != nil {
		cancel()

		return nil, errnie.Error(err)
	}

	orderClient.ledger = ledger

	go func() {
		if runErr := ledger.Run(); runErr != nil {
			errnie.Error(runErr)
		}
	}()

	return orderClient, errnie.Error(errnie.Require(map[string]any{
		"ctx":    orderClient.ctx,
		"cancel": orderClient.cancel,
		"orders": orderClient.orders,
		"ledger": orderClient.ledger,
	}))
}

func (client *Client) Halted() bool {
	return client.ledger.Halted()
}

/*
send publishes an order frame on the private broadcast.
*/
func (client *Client) send(payload fiber.Map) error {
	client.orders.Send(&qpool.QValue[any]{
		Type:  public.OrdersChannel,
		Value: payload,
	})

	return nil
}

func (client *Client) AddOrder(params AddParams) (<-chan OrderResult, error) {
	if params.ClOrdID == "" {
		return nil, errnie.Error(errnie.Require(map[string]any{
			"params.ClOrdID": params.ClOrdID,
		}))
	}

	if client.Halted() {
		return nil, errnie.Error(errnie.Require(map[string]any{
			"halted": client.Halted(),
		}))
	}

	resultCh := client.ledger.Register(params.ClOrdID)

	if err := client.send(fiber.Map{
		"method": MethodAddOrder,
		"params": params,
	}); err != nil {
		return nil, errnie.Error(err)
	}

	return resultCh, nil
}

func (client *Client) AmendOrder(params AmendParams) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodAmendOrder,
		"params": params,
	}))
}

func (client *Client) CancelOrder(params CancelParams) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodCancelOrder,
		"params": params,
	}))
}

func (client *Client) CancelAll(params CancelAllParams) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodCancelAll,
		"params": params,
	}))
}

func (client *Client) CancelAllOrdersAfter(params CancelAllOrdersAfterParams) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodCancelAllOrdersAfter,
		"params": params,
	}))
}

func (client *Client) BatchAdd(params BatchAddParams) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodBatchAdd,
		"params": params,
	}))
}

func (client *Client) BatchCancel(params BatchCancelParams) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodBatchCancel,
		"params": params,
	}))
}

func (client *Client) EditOrder(params EditParams) error {
	return errnie.Error(client.send(fiber.Map{
		"method": MethodEditOrder,
		"params": params,
	}))
}

func (client *Client) Close() error {
	client.cancel()

	if client.ledger != nil {
		return client.ledger.Close()
	}

	return nil
}
