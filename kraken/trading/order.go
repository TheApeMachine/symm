package trading

import (
	"time"

	"github.com/spf13/viper"
)

/*
EntryTransitTTL is how long a market/limit ENTRY stays trustworthy between the
desk's decision and the venue's ears. Five seconds default: the signals that
justify entries (ignition, flow dominance, flash dips) live on that scale, and
nobody should trust a market entry ten seconds old. Both the live private
socket and the paper emulator enforce it, so transit staleness behaves the
same in every trading model.
*/
func EntryTransitTTL() time.Duration {
	configured := viper.GetDuration("trading.entry.transit_ttl")

	if configured <= 0 {
		return 5 * time.Second
	}

	return configured
}

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
	Iceberg           OrderType = "iceberg"
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

	// EntryQueuedAt is when the desk decided this ENTRY — zero for exits, which
	// must survive any delay (an exit is valid forever; an entry is not). It
	// rides with the params but never reaches Kraken: the delivery layer holds
	// frames across reconnects and queue pressure, and a market entry written
	// long after its signal executes a decision whose justification is dead —
	// on a pump, that buys the top. The transit gate drops entries older than
	// trading.entry.transit_ttl instead of writing them.
	EntryQueuedAt time.Time `json:"-"`
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
	Validate bool        `json:"validate,omitempty"`
	Token    string      `json:"token,omitempty"`
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

type OrderUpdate struct {
	OrderID      string `json:"order_id"`
	OrderUserref int    `json:"order_userref"`
}
