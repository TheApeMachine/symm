package kraken

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

type Level3 struct {
	Channel string       `json:"channel"`
	Type    string       `json:"type"`
	Data    []Level3Data `json:"data"`
}

type Level3Data struct {
	Symbol    string        `json:"symbol"`
	Type      string        `json:"type"`
	Timestamp time.Time     `json:"timestamp"`
	Checksum  uint32        `json:"checksum"`
	Bids      []Level3Order `json:"bids"`
	Asks      []Level3Order `json:"asks"`
}

type Level3Order struct {
	Event      string           `json:"event,omitempty"`
	OrderID    string           `json:"order_id"`
	LimitPrice *decimal.Decimal `json:"limit_price"`
	OrderQty   *decimal.Decimal `json:"order_qty"`
	Timestamp  time.Time        `json:"timestamp"`
	limitPrice string
	orderQty   string
}

func (order Level3Order) MarshalJSON() ([]byte, error) {
	wire := struct {
		Event      string          `json:"event,omitempty"`
		OrderID    string          `json:"order_id"`
		LimitPrice json.RawMessage `json:"limit_price"`
		OrderQty   json.RawMessage `json:"order_qty"`
		Timestamp  time.Time       `json:"timestamp"`
	}{
		Event:     order.Event,
		OrderID:   order.OrderID,
		Timestamp: order.Timestamp,
	}

	if order.LimitPrice == nil {
		wire.LimitPrice = json.RawMessage("null")
	}

	if order.LimitPrice != nil {
		limitPriceText := order.limitPrice

		if limitPriceText == "" {
			limitPriceText = order.LimitPrice.String()
		}

		wire.LimitPrice = json.RawMessage(limitPriceText)
	}

	if order.OrderQty == nil {
		wire.OrderQty = json.RawMessage("null")
	}

	if order.OrderQty != nil {
		orderQtyText := order.orderQty

		if orderQtyText == "" {
			orderQtyText = order.OrderQty.String()
		}

		wire.OrderQty = json.RawMessage(orderQtyText)
	}

	return sonic.Marshal(wire)
}

/*
UnmarshalJSON retains Kraken's exact fixed-point price and quantity for both
book arithmetic and checksum construction. A float64 cannot represent every
monetary value or retain the trailing zeroes Kraken includes in its L3 checksum.
*/
func (order *Level3Order) UnmarshalJSON(data []byte) error {
	wire := struct {
		Event      string                 `json:"event,omitempty"`
		OrderID    string                 `json:"order_id"`
		LimitPrice sonic.NoCopyRawMessage `json:"limit_price"`
		OrderQty   sonic.NoCopyRawMessage `json:"order_qty"`
		Timestamp  time.Time              `json:"timestamp"`
	}{}

	if err := sonic.Unmarshal(data, &wire); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "invalid level3 order", err,
		))
	}

	limitPrice, limitPriceText, err := parseLevel3Decimal(wire.LimitPrice)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "level3 limit_price", err,
		))
	}

	orderQty, orderQtyText, err := parseLevel3Decimal(wire.OrderQty)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "level3 order_qty", err,
		))
	}

	order.Event = wire.Event
	order.OrderID = wire.OrderID
	order.LimitPrice = limitPrice
	order.OrderQty = orderQty
	order.Timestamp = wire.Timestamp
	order.limitPrice = limitPriceText
	order.orderQty = orderQtyText
	return nil
}

/*
Resting reports whether this order describes liquidity that is still displayed
on the book after the message carrying it.

A "delete" event reports the order being REMOVED, so its limit_price and
order_qty describe liquidity that is gone: counting it as displayed size reads
withdrawn size as resting size, and because a delete can be priced anywhere —
including through the opposite side's last known touch — it can also
manufacture a crossed book out of a healthy one.
*/
func (order Level3Order) Resting() bool {
	if order.Event == "delete" || order.LimitPrice == nil || order.OrderQty == nil {
		return false
	}

	price := order.LimitPrice.Float64()
	qty := order.OrderQty.Float64()

	return price > 0 && qty > 0 && !math.IsNaN(price) && !math.IsNaN(qty) && !math.IsInf(price, 0) && !math.IsInf(qty, 0)
}

/*
ChecksumLimitPrice returns the exact fixed-point limit_price received from
Kraken. It is intentionally separate from the numerical calculation field.
*/
func (order Level3Order) ChecksumLimitPrice() string {
	return order.limitPrice
}

/*
ChecksumOrderQty returns the exact fixed-point order_qty received from Kraken.
*/
func (order Level3Order) ChecksumOrderQty() string {
	return order.orderQty
}

func parseLevel3Decimal(
	raw []byte,
) (*decimal.Decimal, string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, "", nil
	}

	text := string(raw)

	if raw[0] == '"' {
		if err := sonic.Unmarshal(raw, &text); err != nil {
			return nil, "", errnie.Error(errnie.Err(
				errnie.UnprocessableContent, "invalid level3 decimal", err,
			))
		}
	}

	if strings.ContainsAny(text, "eE") {
		expanded, err := expandLevel3Decimal(text)

		if err != nil {
			return nil, "", errnie.Error(errnie.Err(
				errnie.UnprocessableContent, "invalid level3 decimal", err,
			))
		}

		text = expanded
	}

	value, err := decimal.NewFromString(text)

	if err != nil {
		return nil, "", errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "invalid level3 decimal", err,
		))
	}

	return value, text, nil
}

func expandLevel3Decimal(text string) (string, error) {
	exponentAt := strings.IndexAny(text, "eE")

	if exponentAt < 1 || exponentAt == len(text)-1 {
		return "", errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid scientific decimal",
			fmt.Errorf("invalid scientific decimal %q", text),
		))
	}

	exponent, err := strconv.Atoi(text[exponentAt+1:])

	if err != nil {
		return "", errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid scientific decimal",
			fmt.Errorf("invalid scientific decimal %q: %w", text, err),
		))
	}

	mantissa := text[:exponentAt]
	fractionDigits := 0

	if decimalAt := strings.IndexByte(mantissa, '.'); decimalAt >= 0 {
		fractionDigits = len(mantissa) - decimalAt - 1
	}

	scale := fractionDigits - exponent

	if scale < 0 {
		scale = 0
	}

	rational, valid := new(big.Rat).SetString(text)

	if !valid {
		return "", errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid scientific decimal",
			fmt.Errorf("invalid scientific decimal %q", text),
		))
	}

	return rational.FloatString(scale), nil
}

func NewLevel3(buf []byte) *Level3 {
	var level3 Level3

	if err := sonic.Unmarshal(buf, &level3); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid level3",
			err,
		))
	}

	for index := range level3.Data {
		if level3.Data[index].Type == "" {
			level3.Data[index].Type = level3.Type
		}
	}

	return &level3
}

func (level3 *Level3) Action() string {
	return "level3"
}
