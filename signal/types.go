package signal

import (
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Input struct {
	Role   string
	At     time.Time
	Ticker kraken.TickerDataSlice
	Trade  kraken.TradeDataSlice
	OHLC   kraken.OHLCDataSlice
	Book   kraken.BookDataSlice
	Level3 kraken.Level3DataSlice
}

type Measurement struct {
	Origin  logic.SourceType
	Role    string
	Symbol  string
	At      time.Time
	Output  map[string]float64
	Mass    map[logic.CategoryType]float64
	Strings map[string]string
}

func NewMeasurement(
	origin logic.SourceType,
	symbol string,
	at time.Time,
) Measurement {
	return Measurement{
		Origin: origin,
		Role:   "measurement",
		Symbol: symbol,
		At:     at,
		Output: map[string]float64{},
		Mass:   map[logic.CategoryType]float64{},
	}
}

func (measurement Measurement) Complete() bool {
	return measurement.Symbol != "" &&
		!measurement.At.IsZero() &&
		measurement.Output["value"] > 0 &&
		measurement.Output["confidence"] > 0 &&
		measurement.Output["entry_baseline"] > 0 &&
		measurement.Output["exit_baseline"] > 0
}

func (measurement *Measurement) Merge(result map[string]any) {
	for key, value := range result {
		switch typed := value.(type) {
		case float64:
			measurement.Output[key] = typed
		case []float64:
			for index, item := range typed {
				measurement.Output[key+"_"+string(rune('0'+index))] = item
			}
		case map[string]float64:
			for category, probability := range typed {
				measurement.Output["distribution_"+category] = probability
			}
		}
	}
}
