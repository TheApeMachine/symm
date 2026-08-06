package kraken

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

type Instrument struct {
	Channel string         `json:"channel"`
	Type    string         `json:"type"`
	Data    InstrumentData `json:"data"`
}

func NewInstrument(buf []byte) *Instrument {
	instrument := &Instrument{}
	errnie.Error(sonic.Unmarshal(buf, instrument))
	return instrument
}

type InstrumentData struct {
	Pairs []InstrumentPair `json:"pairs"`
}

/*
InstrumentPair carries Kraken's execution rules. Fixed-point price, cost, and
quantity boundaries remain Decimal so sizing never reconstructs venue rules
from binary floating-point values.
*/
type InstrumentPair struct {
	Symbol             string           `json:"symbol"`
	Base               string           `json:"base"`
	Quote              string           `json:"quote"`
	Status             string           `json:"status"`
	QtyPrecision       int              `json:"qty_precision"`
	QtyIncrement       *decimal.Decimal `json:"qty_increment"`
	PricePrecision     int              `json:"price_precision"`
	CostPrecision      int              `json:"cost_precision"`
	Marginable         bool             `json:"marginable"`
	HasIndex           bool             `json:"has_index"`
	CostMin            *decimal.Decimal `json:"cost_min"`
	MarginInitial      decimal.Decimal  `json:"margin_initial"`
	PositionLimitLong  int              `json:"position_limit_long"`
	PositionLimitShort int              `json:"position_limit_short"`
	TickSize           decimal.Decimal  `json:"tick_size"`
	PriceIncrement     decimal.Decimal  `json:"price_increment"`
	QtyMin             *decimal.Decimal `json:"qty_min"`
}

type InstrumentSubscription struct {
	Method string                       `json:"method"`
	Params InstrumentSubscriptionParams `json:"params"`
}

type InstrumentSubscriptionParams struct {
	Channel string `json:"channel"`
}

func NewInstrumentSubscription() InstrumentSubscription {
	return InstrumentSubscription{
		Method: "subscribe",
		Params: InstrumentSubscriptionParams{
			Channel: "instrument",
		},
	}
}

func (subscription InstrumentSubscription) MarshalJSON() ([]byte, error) {
	type alias InstrumentSubscription
	return sonic.Marshal((*alias)(&subscription))
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
