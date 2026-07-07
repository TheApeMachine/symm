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
	Symbol         string          `json:"symbol"`
	Status         string          `json:"status"`
	PriceIncrement decimal.Decimal `json:"price_increment"`
	TickSize       decimal.Decimal `json:"tick_size"`
}

func NewInstrumentData(buf []byte) InstrumentData {
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
