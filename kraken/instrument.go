package kraken

import (
	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
)

type Instrument struct {
	Channel string `json:"channel"`
	Type    string `json:"type"`
	Data    struct {
		Pairs []InstrumentPair `json:"pairs"`
	} `json:"data"`
}

func NewInstrument(buf []byte) *Instrument {
	instrument := &Instrument{}

	if err := sonic.Unmarshal(buf, instrument); err != nil {
		errnie.Error(errnie.Err(errnie.UnprocessableContent, "[kraken.instrument] snapshot decode failed", err))
		return nil
	}

	return instrument
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
	MarginInitial      *decimal.Decimal `json:"margin_initial"`
	PositionLimitLong  int              `json:"position_limit_long"`
	PositionLimitShort int              `json:"position_limit_short"`
	TickSize           *decimal.Decimal `json:"tick_size"`
	PriceIncrement     *decimal.Decimal `json:"price_increment"`
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

/*
ToValue serializes the Instrument snapshot into a *data.Value layer.
*/
func (instrument *Instrument) ToValue(epochs ...int64) *data.Value {
	if instrument == nil {
		return nil
	}

	val := data.NewValue("kraken", "snapshot", "instrument")
	if len(epochs) > 0 && epochs[0] > 0 {
		val.Epoch = uint64(epochs[0])
	}
	val.Append(instrument)
	return val
}

/*
NewInstrumentFromValue reconstructs an Instrument from a *data.Value.
*/
func NewInstrumentFromValue(val *data.Value) (*Instrument, error) {
	if val == nil || len(val.Layers) == 0 {
		return nil, errnie.Err(errnie.Validation, "[kraken.instrument] cannot convert nil value to instrument", nil)
	}

	var instrument Instrument
	if err := sonic.Unmarshal(val.Layers[0], &instrument); err != nil {
		return nil, errnie.Err(errnie.Validation, "[kraken.instrument] failed to unmarshal instrument", err)
	}

	return &instrument, nil
}

/*
ToValue serializes the InstrumentPair into a *data.Value layer.
*/
func (pair *InstrumentPair) ToValue(epochs ...int64) *data.Value {
	if pair == nil {
		return nil
	}

	val := data.NewValue("kraken", "instrument", pair.Symbol)
	if len(epochs) > 0 && epochs[0] > 0 {
		val.Epoch = uint64(epochs[0])
	}
	val.Append(pair)
	return val
}

/*
NewInstrumentPairFromValue reconstructs an InstrumentPair from a *data.Value.
*/
func NewInstrumentPairFromValue(val *data.Value) (*InstrumentPair, error) {
	if val == nil || len(val.Layers) == 0 {
		return nil, errnie.Err(errnie.Validation, "[kraken.instrument] cannot convert nil value to pair", nil)
	}

	var pair InstrumentPair
	if err := sonic.Unmarshal(val.Layers[0], &pair); err != nil {
		return nil, errnie.Err(errnie.Validation, "[kraken.instrument] failed to unmarshal pair", err)
	}

	return &pair, nil
}
