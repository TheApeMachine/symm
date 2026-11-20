package kraken

import (
	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
)

/*
Instrument is the ws instrument snapshot channel for pair metadata at boot.
*/
type Instrument struct {
	Channel string `json:"channel"`
	Type    string `json:"type"`
	Data    struct {
		Pairs []InstrumentPair `json:"pairs"`
	} `json:"data"`
}

/*
NewInstrument decodes one instrument frame for sizing rules before L3 subscribe.
*/
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

/*
InstrumentSubscription is the public ws subscribe envelope for instrument.
*/
type InstrumentSubscription struct {
	Method string                       `json:"method"`
	Params InstrumentSubscriptionParams `json:"params"`
}

/*
InstrumentSubscriptionParams names the instrument channel on subscribe.
*/
type InstrumentSubscriptionParams struct {
	Channel string `json:"channel"`
}

/*
NewInstrumentSubscription builds the default instrument subscribe frame.
*/
func NewInstrumentSubscription() InstrumentSubscription {
	return InstrumentSubscription{
		Method: "subscribe",
		Params: InstrumentSubscriptionParams{
			Channel: "instrument",
		},
	}
}

/*
MarshalJSON aliases InstrumentSubscription for sonic encode at dial time.
*/
func (subscription InstrumentSubscription) MarshalJSON() ([]byte, error) {
	type alias InstrumentSubscription
	return sonic.Marshal((*alias)(&subscription))
}
