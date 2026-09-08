package strategy

import (
	"fmt"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
)

const rawMarketSource = "market"

/*
measurementFromObservation projects one historical market observation into the
same numerical measurement shape the live envelope produces. It carries only
facts the venue record actually defined: absent quotes and absent trades stay
absent instead of becoming zeros.
*/
func measurementFromObservation(observation hindsight.Observation) *data.Measurement[float64] {
	at := observation.At()
	measurement := data.NewMeasurement[float64](
		fmt.Sprintf("market:%s:%d:%d", observation.Symbol, observation.Capture.Sequence, observation.Ordinal),
		observation.Symbol,
		rawMarketSource,
		at,
		at,
	)

	if observation.HasBid && observation.Bid > 0 {
		measurement.PutMetric(data.Metric[float64]{Label: "bid", Raw: observation.Bid, Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous})
	}

	if observation.HasAsk && observation.Ask > 0 {
		measurement.PutMetric(data.Metric[float64]{Label: "ask", Raw: observation.Ask, Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous})
	}

	if observation.HasLast && observation.Last > 0 {
		measurement.PutMetric(data.Metric[float64]{Label: "last", Raw: observation.Last, Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous})
	}

	if observation.HasTrade && observation.TradePrice > 0 {
		measurement.PutMetric(data.Metric[float64]{Label: "trade", Raw: observation.TradePrice, Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous})
	}

	if spread, defined := observation.SpreadFraction(); defined {
		measurement.PutMetric(data.Metric[float64]{Label: "spread", Raw: spread, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}

	if depth, defined := observation.TouchDepth(); defined {
		measurement.PutMetric(data.Metric[float64]{Label: "depth", Raw: depth, Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous})
	}

	return measurement
}
