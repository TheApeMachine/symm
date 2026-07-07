package kraken

import (
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
)

type Order struct {
	Method string `json:"method"`
	Params any    `json:"params"`
	ReqID  int    `json:"req_id"`
}

type OrderResponseResult struct {
	OrderID      string   `json:"order_id"`
	ClOrdID      string   `json:"cl_ord_id"`
	OrderUserRef int64    `json:"order_userref"`
	Warnings     []string `json:"warnings"`
}

type OrderResponse struct {
	Method  string              `json:"method"`
	Result  OrderResponseResult `json:"result"`
	Error   string              `json:"error"`
	Success bool                `json:"success"`
	ReqID   int                 `json:"req_id"`
	TimeIn  time.Time           `json:"time_in"`
	TimeOut time.Time           `json:"time_out"`
}

func NewOrderResponse(buf []byte) *OrderResponse {
	data := &OrderResponse{}
	errnie.Error(sonic.Unmarshal(buf, data))
	return data
}

type OrderDescription struct {
	Pair     string          `json:"pair"`
	Side     string          `json:"type"`
	Type     string          `json:"ordertype"`
	Price    decimal.Decimal `json:"price"`
	Price2   decimal.Decimal `json:"price2"`
	Leverage string          `json:"leverage"`
	Order    string          `json:"order"`
	Close    string          `json:"close"`
}

type OrderData struct {
	ID             string           `json:"id"`
	RefID          string           `json:"refid"`
	ClOrdID        string           `json:"cl_ord_id"`
	UserRef        decimal.Decimal  `json:"userref"`
	Status         string           `json:"status"`
	OpenTime       decimal.Decimal  `json:"opentm"`
	StartTime      decimal.Decimal  `json:"starttm"`
	ExpireTime     decimal.Decimal  `json:"expiretm"`
	Description    OrderDescription `json:"descr"`
	Pair           string           `json:"pair"`
	Price          decimal.Decimal  `json:"price"`
	ReservedAmount decimal.Decimal  `json:"reserved_amount"`
	ReservedAsset  string           `json:"reserved_asset"`
	Side           string           `json:"side"`
	Type           string           `json:"type"`
	Volume         decimal.Decimal  `json:"volume"`
	VolumeExecuted decimal.Decimal  `json:"vol_exec"`
	Cost           decimal.Decimal  `json:"cost"`
	Fee            decimal.Decimal  `json:"fee"`
	StopPrice      decimal.Decimal  `json:"stopprice"`
	LimitPrice     decimal.Decimal  `json:"limitprice"`
	Misc           string           `json:"misc"`
	OrderFlags     string           `json:"oflags"`
	Trades         []string         `json:"trades"`
	CreatedAt      string           `json:"created_at"`
}

type OrderDataSlice []OrderData

func NewOrderDataSlice(buf []byte) *OrderDataSlice {
	data := &OrderDataSlice{}
	errnie.Error(sonic.Unmarshal(buf, data))
	return data
}

func NewOrderDataSliceFromSpot(orders map[string]spot.Order) OrderDataSlice {
	ids := make([]string, 0, len(orders))

	for id := range orders {
		ids = append(ids, id)
	}

	sort.Strings(ids)
	rows := make(OrderDataSlice, 0, len(ids))

	for _, id := range ids {
		rows = append(rows, NewOrderDataFromSpot(id, orders[id]))
	}

	return rows
}

func NewOrderDataFromSpot(id string, order spot.Order) OrderData {
	row := OrderData{
		ID:         id,
		RefID:      order.RefID,
		ClOrdID:    order.ClOrdID,
		Status:     order.Status,
		Misc:       order.Misc,
		OrderFlags: order.OrderFlags,
		Trades:     order.Trades,
	}

	if order.UserRef != nil {
		row.UserRef = *order.UserRef
	}

	if order.OpenTm != nil {
		row.OpenTime = *order.OpenTm
		row.CreatedAt = order.OpenTm.String()
	}

	if order.StartTm != nil {
		row.StartTime = *order.StartTm
	}

	if order.ExpireTm != nil {
		row.ExpireTime = *order.ExpireTm
	}

	if order.Description != nil {
		row.Description = NewOrderDescriptionFromSpot(*order.Description)
		row.Pair = row.Description.Pair
		row.Side = row.Description.Side
		row.Type = row.Description.Type
		row.Price = row.Description.Price
	}

	if order.Volume != nil {
		row.Volume = *order.Volume
	}

	if order.VolumeExecuted != nil {
		row.VolumeExecuted = *order.VolumeExecuted
		scale := row.Volume.GetScale()
		if row.VolumeExecuted.GetScale() > scale {
			scale = row.VolumeExecuted.GetScale()
		}

		volume := row.Volume.SetScale(scale)
		row.Volume = *volume.Sub(row.VolumeExecuted.SetScale(scale))
	}

	if order.Cost != nil {
		row.Cost = *order.Cost
	}

	if order.Fee != nil {
		row.Fee = *order.Fee
	}

	if order.Price != nil {
		row.Price = *order.Price
	}

	if order.StopPrice != nil {
		row.StopPrice = *order.StopPrice
	}

	if order.LimitPrice != nil {
		row.LimitPrice = *order.LimitPrice
	}

	row.ReservedAsset, row.ReservedAmount = row.Reservation()
	return row
}

func NewOrderDescriptionFromSpot(description spot.OrderDescription) OrderDescription {
	row := OrderDescription{
		Pair:     description.Pair,
		Side:     description.Type,
		Type:     description.OrderType,
		Leverage: description.Leverage,
		Order:    description.Order,
		Close:    description.Close,
	}

	if description.Price != nil {
		row.Price = *description.Price
	}

	if description.SecondaryPrice != nil {
		row.Price2 = *description.SecondaryPrice
	}

	return row
}

func (order OrderData) Reservation() (string, decimal.Decimal) {
	pair := strings.TrimSpace(order.Pair)
	base := pair
	quote := ""

	if cutBase, cutQuote, ok := strings.Cut(pair, "/"); ok {
		base = cutBase
		quote = cutQuote
	}

	if quote == "" {
		return "", decimal.Decimal{}
	}

	if strings.EqualFold(order.Side, "sell") {
		return base, order.Volume
	}

	scale := order.Price.GetScale()
	if order.Volume.GetScale() > scale {
		scale = order.Volume.GetScale()
	}

	price := order.Price.SetScale(scale)
	return quote, *price.Mul(order.Volume.SetScale(scale))
}

type LimitOrderParams struct {
	OrderType    string  `json:"order_type"`
	Side         string  `json:"side"`
	LimitPrice   float64 `json:"limit_price"`
	OrderUserref int     `json:"order_userref"`
	OrderQty     float64 `json:"order_qty"`
	Symbol       string  `json:"symbol"`
	Token        string  `json:"token"`
}

type StoplossOrderParams struct {
	OrderType string        `json:"order_type"`
	Side      string        `json:"side"`
	OrderQty  int           `json:"order_qty"`
	Symbol    string        `json:"symbol"`
	Triggers  TriggerParams `json:"triggers"`
	Token     string        `json:"token"`
}

type TriggerParams struct {
	Reference string  `json:"reference"`
	Price     float64 `json:"price"`
	PriceType string  `json:"price_type"`
}

func (order *Order) Marshal() []byte {
	buf, err := sonic.Marshal(order)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	return buf
}
