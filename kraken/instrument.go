package kraken

import (
	"math"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

type Instrument struct {
	Channel string         `json:"channel"`
	Type    string         `json:"type"`
	Data    InstrumentData `json:"data"`
}

type InstrumentData struct {
	Pairs []InstrumentPair `json:"pairs"`
}

type InstrumentPair struct {
	Symbol             string          `json:"symbol"`
	Base               string          `json:"base"`
	Quote              string          `json:"quote"`
	Status             string          `json:"status"`
	QtyPrecision       int             `json:"qty_precision"`
	QtyIncrement       float64         `json:"qty_increment"`
	PricePrecision     int             `json:"price_precision"`
	CostPrecision      int             `json:"cost_precision"`
	Marginable         bool            `json:"marginable"`
	HasIndex           bool            `json:"has_index"`
	CostMin            decimal.Decimal `json:"cost_min"`
	MarginInitial      decimal.Decimal `json:"margin_initial"`
	PositionLimitLong  int             `json:"position_limit_long"`
	PositionLimitShort int             `json:"position_limit_short"`
	TickSize           decimal.Decimal `json:"tick_size"`
	PriceIncrement     decimal.Decimal `json:"price_increment"`
	QtyMin             float64         `json:"qty_min"`
}

type InstrumentSubscription struct{}

func NewInstrumentSubscription() InstrumentSubscription {
	return InstrumentSubscription{}
}

func (subscription InstrumentSubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "instrument",
		},
	})
}

func NewInstrumentData(buf []byte) InstrumentData {
	data := InstrumentData{}

	if err := sonic.Unmarshal(buf, &data); err == nil && len(data.Pairs) > 0 {
		return data
	}

	frame := Instrument{}
	errnie.Error(sonic.Unmarshal(buf, &frame))

	return frame.Data
}

func (pair InstrumentPair) Increment() decimal.Decimal {
	if decimalPositive(pair.PriceIncrement) {
		return pair.PriceIncrement
	}

	return pair.TickSize
}

func (pair InstrumentPair) HasIncrement() bool {
	return decimalPositive(pair.Increment())
}

/*
RoundQty floors quantity onto the exchange lot grid using QtyIncrement when
present, otherwise QtyPrecision decimal places.
*/
func (pair InstrumentPair) RoundQty(quantity *decimal.Decimal) *decimal.Decimal {
	if quantity == nil {
		return nil
	}

	if pair.QtyIncrement > 0 {
		steps := math.Floor(quantity.Float64() / pair.QtyIncrement)
		rounded := decimal.NewFromFloat64(steps * pair.QtyIncrement)

		if pair.QtyPrecision > 0 {
			return rounded.SetScale(int64(pair.QtyPrecision))
		}

		return rounded
	}

	if pair.QtyPrecision > 0 {
		return quantity.Copy().SetScale(int64(pair.QtyPrecision))
	}

	return quantity.Copy()
}

/*
MeetsCostMin reports whether a quote notional clears the instrument cost floor.
*/
func (pair InstrumentPair) MeetsCostMin(notional *decimal.Decimal) bool {
	if notional == nil {
		return false
	}

	if !decimalPositive(pair.CostMin) {
		return notional.Sign() > 0
	}

	return notional.Cmp(&pair.CostMin) >= 0
}

func decimalPositive(value decimal.Decimal) (positive bool) {
	defer func() {
		if recover() != nil {
			positive = false
		}
	}()

	return value.Sign() > 0
}
