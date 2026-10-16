package kraken

import (
	"encoding/json"
	"fmt"
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
	// Raw is the frame exactly as received, kept for failure diagnostics.
	Raw []byte `json:"-"`
}

type Level3Data struct {
	Symbol    string        `json:"symbol"`
	Type      string        `json:"type"`
	Timestamp time.Time     `json:"timestamp"`
	Checksum  uint32        `json:"checksum"`
	Bids      []Level3Order `json:"bids"`
	Asks      []Level3Order `json:"asks"`
	// Sequence is the first ingress sequence number of this entry's orders,
	// bids then asks, taken by the book once the entry is applied. Zero means
	// the book did not accept the entry.
	Sequence int64 `json:"-"`
}

type Level3Order struct {
	Event      string           `json:"event,omitempty"`
	OrderID    string           `json:"order_id"`
	LimitPrice *decimal.Decimal `json:"limit_price"`
	OrderQty   *decimal.Decimal `json:"order_qty"`
	Timestamp  time.Time        `json:"timestamp"`
}

/*
UnmarshalJSON decodes limit_price and order_qty from the exact number text
Kraken sent. The level3 checksum hashes those digits, so they are never
routed through a float: plain decimals keep every digit (trailing zeros
included), and scientific notation (Kraken sends e.g. 1e-05 for a minimum
lot) is expanded digit for digit to its plain decimal form.
*/
func (order *Level3Order) UnmarshalJSON(buf []byte) error {
	var wire struct {
		Event      string          `json:"event,omitempty"`
		OrderID    string          `json:"order_id"`
		LimitPrice json.RawMessage `json:"limit_price"`
		OrderQty   json.RawMessage `json:"order_qty"`
		Timestamp  time.Time       `json:"timestamp"`
	}

	if err := sonic.Unmarshal(buf, &wire); err != nil {
		return err
	}

	price, err := exactDecimal(wire.LimitPrice)

	if err != nil {
		return fmt.Errorf("level3 order %s limit_price: %w", wire.OrderID, err)
	}

	quantity, err := exactDecimal(wire.OrderQty)

	if err != nil {
		return fmt.Errorf("level3 order %s order_qty: %w", wire.OrderID, err)
	}

	*order = Level3Order{
		Event:      wire.Event,
		OrderID:    wire.OrderID,
		LimitPrice: price,
		OrderQty:   quantity,
		Timestamp:  wire.Timestamp,
	}

	return nil
}

/*
exactDecimal parses one JSON number (or numeric string) without losing a
digit. An absent or null field is nil: a delete may omit its price.
*/
func exactDecimal(raw json.RawMessage) (*decimal.Decimal, error) {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)

	if text == "" || text == "null" {
		return nil, nil
	}

	plain, err := plainDecimal(text)

	if err != nil {
		return nil, err
	}

	return decimal.NewFromString(plain)
}

/*
plainDecimal rewrites scientific notation (1.50e-05) as a plain decimal
(0.0000150) by moving the decimal point over the mantissa's own digits.
*/
func plainDecimal(text string) (string, error) {
	mantissa, exponentText, scientific := strings.Cut(strings.ToLower(text), "e")

	if !scientific {
		return text, nil
	}

	exponent, err := strconv.Atoi(exponentText)

	if err != nil {
		return "", fmt.Errorf("invalid exponent in %q: %w", text, err)
	}

	sign := ""

	if strings.HasPrefix(mantissa, "-") || strings.HasPrefix(mantissa, "+") {
		if mantissa[0] == '-' {
			sign = "-"
		}

		mantissa = mantissa[1:]
	}

	integer, fraction, _ := strings.Cut(mantissa, ".")
	digits := integer + fraction

	if digits == "" || strings.Trim(digits, "0123456789") != "" {
		return "", fmt.Errorf("invalid mantissa in %q", text)
	}

	point := len(integer) + exponent

	switch {
	case point <= 0:
		return sign + "0." + strings.Repeat("0", -point) + digits, nil
	case point >= len(digits):
		return sign + digits + strings.Repeat("0", point-len(digits)), nil
	default:
		return sign + digits[:point] + "." + digits[point:], nil
	}
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

	return order.LimitPrice.Sign() > 0 && order.OrderQty.Sign() > 0
}

/*
ChecksumLimitPrice returns the fixed-point limit_price received from Kraken.
*/
func (order Level3Order) ChecksumLimitPrice() string {
	if order.LimitPrice == nil {
		return ""
	}
	return order.LimitPrice.String()
}

/*
ChecksumOrderQty returns the fixed-point order_qty received from Kraken.
*/
func (order Level3Order) ChecksumOrderQty() string {
	if order.OrderQty == nil {
		return ""
	}
	return order.OrderQty.String()
}

/*
NewLevel3 decodes one venue level3 frame. A frame that cannot be decoded is an
error, never a partial or empty frame: a lost delta leaves the local book
silently wrong, so the caller must halt rather than apply what was parsed.
*/
func NewLevel3(buf []byte) *Level3 {
	var level3 Level3

	if err := sonic.Unmarshal(buf, &level3); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid level3",
			err,
		))

		return nil
	}

	level3.Raw = append([]byte(nil), buf...)

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

/*
Level3Touch is the executable top of book at the moment a Level 3 frame was
accepted, recorded as its own observation.

It exists because the venue's ticker stream cannot serve as a price series for
most of the universe. Kraken publishes spot ticker on change, which for an
illiquid pair is a handful of frames across an entire session — far under what
any estimator needs before it will look at a symbol at all. The Level 3 book
carries the same price continuously, and it is the price the desk would
actually trade at rather than a summary of it.

The touch is only meaningful once the local book has matched the venue's own
checksum for that frame. Before that the state is unverified, and a price taken
from unverified depth is not a reading of the market.
*/
type Level3Touch struct {
	Symbol    string           `json:"symbol"`
	Timestamp time.Time        `json:"timestamp"`
	Bid       *decimal.Decimal `json:"bid"`
	BidQty    *decimal.Decimal `json:"bid_qty"`
	Ask       *decimal.Decimal `json:"ask"`
	AskQty    *decimal.Decimal `json:"ask_qty"`
}

/* Level3TouchFrame is one batch of touches carried by a single accepted frame. */
type Level3TouchFrame struct {
	Channel string        `json:"channel"`
	Type    string        `json:"type"`
	Data    []Level3Touch `json:"data"`
}

type Level3Subscription struct {
	Method string                   `json:"method"`
	Params Level3SubscriptionParams `json:"params"`
}

type Level3SubscriptionParams struct {
	Channel string   `json:"channel"`
	Symbol  []string `json:"symbol"`
	Depth   int      `json:"depth,omitempty"`
	Token   string   `json:"token,omitempty"`
}

/*
NewLevel3Subscription builds a Level3 subscribe frame. depth is sent
explicitly so the venue's per-symbol snapshot cost matches the budget the
caller paced for.
*/
func NewLevel3Subscription(symbols []string, token string, depth int) Level3Subscription {
	return Level3Subscription{
		Method: "subscribe",
		Params: Level3SubscriptionParams{
			Channel: "level3",
			Symbol:  symbols,
			Depth:   depth,
			Token:   token,
		},
	}
}

func (subscription Level3Subscription) MarshalJSON() ([]byte, error) {
	type alias Level3Subscription
	return sonic.Marshal((*alias)(&subscription))
}
