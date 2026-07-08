package kraken

import (
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

func decimalPositive(value decimal.Decimal) (positive bool) {
	defer func() {
		if recover() != nil {
			positive = false
		}
	}()

	return value.Sign() > 0
}
