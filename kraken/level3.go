package kraken

import (
	"encoding/json"
	"fmt"
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

/*
Level3Subscription is the authenticated Kraken Level3 subscription envelope.
*/
type Level3Subscription struct {
	Method string                   `json:"method"`
	Params Level3SubscriptionParams `json:"params"`
}

/*
Level3SubscriptionParams identifies the symbols and depth requested from Kraken.
*/
type Level3SubscriptionParams struct {
	Channel string   `json:"channel"`
	Symbol  []string `json:"symbol"`
	Depth   int      `json:"depth"`
}

/*
NewLevel3Subscription creates the same request for live and injected Conns.
*/
func NewLevel3Subscription(symbols []string, depth int) Level3Subscription {
	return Level3Subscription{
		Method: "subscribe",
		Params: Level3SubscriptionParams{
			Channel: "level3",
			Symbol:  symbols,
			Depth:   depth,
		},
	}
}

/*
MarshalJSON implements json.Marshaler for the Conn write boundary.
*/
func (subscription Level3Subscription) MarshalJSON() ([]byte, error) {
	type wire Level3Subscription

	return sonic.Marshal(wire(subscription))
}

/*
UnmarshalJSON retains Kraken's exact fixed-point price and quantity for both
book arithmetic and checksum construction. A float64 cannot represent every
monetary value or retain the trailing zeroes Kraken includes in its L3 checksum.
*/
func (order *Level3Order) UnmarshalJSON(data []byte) error {
	wire := struct {
		Event      string          `json:"event,omitempty"`
		OrderID    string          `json:"order_id"`
		LimitPrice json.RawMessage `json:"limit_price"`
		OrderQty   json.RawMessage `json:"order_qty"`
		Timestamp  time.Time       `json:"timestamp"`
	}{}

	if err := sonic.Unmarshal(data, &wire); err != nil {
		return err
	}

	limitPrice, limitPriceText, err := parseLevel3Decimal(wire.LimitPrice)

	if err != nil {
		return fmt.Errorf("level3 limit_price: %w", err)
	}

	orderQty, orderQtyText, err := parseLevel3Decimal(wire.OrderQty)

	if err != nil {
		return fmt.Errorf("level3 order_qty: %w", err)
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
	raw json.RawMessage,
) (*decimal.Decimal, string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, "", nil
	}

	text := string(raw)

	if raw[0] == '"' {
		if err := sonic.Unmarshal(raw, &text); err != nil {
			return nil, "", err
		}
	}

	value, err := decimal.NewFromString(text)

	if err != nil {
		return nil, "", err
	}

	if strings.ContainsAny(text, "eE") {
		text = value.String()
	}

	return value, text, nil
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

type Level3DataSlice []Level3Data

func NewLevel3DataSlice(buf []byte) Level3DataSlice {
	isArray := false
	for _, b := range buf {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if b == '[' {
			isArray = true
		}
		break
	}

	if isArray {
		data := Level3DataSlice{}
		if err := sonic.Unmarshal(buf, &data); err == nil && len(data) > 0 {
			return data
		}
	}

	frame := Level3{}
	errnie.Error(sonic.Unmarshal(buf, &frame))

	for index := range frame.Data {
		if frame.Data[index].Type == "" {
			frame.Data[index].Type = frame.Type
		}
	}

	return frame.Data
}
