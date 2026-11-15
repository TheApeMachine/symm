package kraken

import (
	"strings"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
)

/*
HistoryOrderDescr is the descr block on OpenOrders / ClosedOrders / QueryOrders.
*/
type HistoryOrderDescr struct {
	Pair      string           `json:"pair"`
	Type      string           `json:"type"`
	OrderType string           `json:"ordertype"`
	Price     *decimal.Decimal `json:"price"`
	Price2    *decimal.Decimal `json:"price2"`
	Leverage  string           `json:"leverage"`
	Order     string           `json:"order"`
	Close     string           `json:"close"`
}

/*
HistoryOrder is one REST order row keyed by txid — OpenOrders, ClosedOrders,
and QueryOrders share this shape (parallel to HistoryTrade).
*/
type HistoryOrder struct {
	RefID            string            `json:"refid"`
	UserRef          int               `json:"userref"`
	ClOrdID          string            `json:"cl_ord_id,omitempty"`
	Status           string            `json:"status"`
	Reason           string            `json:"reason,omitempty"`
	OpenTm           *decimal.Decimal  `json:"opentm"`
	CloseTm          *decimal.Decimal  `json:"closetm,omitempty"`
	StartTm          *decimal.Decimal  `json:"starttm"`
	ExpireTm         *decimal.Decimal  `json:"expiretm"`
	Descr            HistoryOrderDescr `json:"descr"`
	Vol              *decimal.Decimal  `json:"vol"`
	VolExec          *decimal.Decimal  `json:"vol_exec"`
	Cost             *decimal.Decimal  `json:"cost"`
	Fee              *decimal.Decimal  `json:"fee"`
	Price            *decimal.Decimal  `json:"price"`
	StopPrice        *decimal.Decimal  `json:"stopprice"`
	LimitPrice       *decimal.Decimal  `json:"limitprice"`
	DisplayVol       *decimal.Decimal  `json:"displayvol,omitempty"`
	DisplayVolRemain *decimal.Decimal  `json:"displayvolremain,omitempty"`
	Trigger          string            `json:"trigger,omitempty"`
	Margin           bool              `json:"margin,omitempty"`
	ReduceOnly       bool              `json:"reduce_only,omitempty"`
	Misc             string            `json:"misc"`
	OFlags           string            `json:"oflags"`
	ExtOrdID         string            `json:"ext_ord_id,omitempty"`
	LinkID           string            `json:"link_id,omitempty"`
	SenderSubID      string            `json:"sender_sub_id,omitempty"`
	Trades           []string          `json:"trades,omitempty"`
}

/*
Request projects the wire fields Send / owns need.
*/
func (order HistoryOrder) Request() *spot.OrderRequest {
	volume := ""

	if order.Vol != nil {
		volume = order.Vol.String()
	}

	return &spot.OrderRequest{
		ClOrdId:   order.ClOrdID,
		OrderType: order.Descr.OrderType,
		Type:      order.Descr.Type,
		Volume:    volume,
	}
}

/*
QueryOrdersRequest asks for up to 50 txids.
*/
type QueryOrdersRequest struct {
	Txid string `json:"txid"`
}

/*
NewQueryOrdersRequest joins venue order ids for one QueryOrders POST.
*/
func NewQueryOrdersRequest(ids []string) *QueryOrdersRequest {
	return &QueryOrdersRequest{Txid: strings.Join(ids, ",")}
}

/*
OpenOrders is private REST Get Open Orders.
*/
type OpenOrders struct {
	Error  []any `json:"error"`
	Result struct {
		Open map[string]HistoryOrder `json:"open"`
	} `json:"result"`
}

/*
ClosedOrders is private REST Get Closed Orders.
*/
type ClosedOrders struct {
	Error  []any `json:"error"`
	Result struct {
		Closed map[string]HistoryOrder `json:"closed"`
		Count  int                     `json:"count"`
	} `json:"result"`
}

/*
QueryOrders is private REST Query Orders Info — result is the txid map itself.
*/
type QueryOrders struct {
	Error  []any                   `json:"error"`
	Result map[string]HistoryOrder `json:"result"`
}

/*
NewOpenOrdersResult parses an OpenOrders envelope into the open map.
*/
func NewOpenOrdersResult(data []byte) map[string]HistoryOrder {
	var envelope OpenOrders

	if err := sonic.Unmarshal(data, &envelope); err != nil || len(envelope.Error) > 0 {
		errnie.Error(errnie.Err(errnie.IO, "[kraken.OpenOrders] decode failed", err).
			With("error", envelope.Error))
		return nil
	}

	if envelope.Result.Open == nil {
		return map[string]HistoryOrder{}
	}

	return envelope.Result.Open
}

/*
NewClosedOrdersResult parses a ClosedOrders envelope into the closed map.
*/
func NewClosedOrdersResult(data []byte) map[string]HistoryOrder {
	var envelope ClosedOrders

	if err := sonic.Unmarshal(data, &envelope); err != nil || len(envelope.Error) > 0 {
		errnie.Error(errnie.Err(errnie.IO, "[kraken.ClosedOrders] decode failed", err).
			With("error", envelope.Error))
		return nil
	}

	if envelope.Result.Closed == nil {
		return map[string]HistoryOrder{}
	}

	return envelope.Result.Closed
}

/*
NewQueryOrdersResult parses a QueryOrders envelope into the txid map.
*/
func NewQueryOrdersResult(data []byte) map[string]HistoryOrder {
	var envelope QueryOrders

	if err := sonic.Unmarshal(data, &envelope); err != nil || len(envelope.Error) > 0 {
		errnie.Error(errnie.Err(errnie.IO, "[kraken.QueryOrders] decode failed", err).
			With("error", envelope.Error))
		return nil
	}

	if envelope.Result == nil {
		return map[string]HistoryOrder{}
	}

	return envelope.Result
}
