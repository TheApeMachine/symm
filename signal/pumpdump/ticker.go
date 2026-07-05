package pumpdump

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Ticker struct {
	ignition   *equation.Ignition
	classifier *probability.ScoreClassifier
}

func NewTicker() *Ticker {
	return &Ticker{
		ignition: equation.NewIgnition(),
		classifier: probability.NewScoreClassifier(
			[]string{"ignition", "compression", "trend", "exhaustion"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryVerticalIgnition)),
				float64(logic.CategoryIndex(logic.CategoryCoiledCompression)),
				float64(logic.CategoryIndex(logic.CategoryOrganicTrend)),
				float64(logic.CategoryIndex(logic.CategoryFadedExhaustion)),
			},
		),
	}
}

func (ticker *Ticker) Measure(row kraken.TickerData) (*logic.Measurement, error) {
	output, ready, err := ticker.ignition.Measure(equation.IgnitionInput{
		Symbol: row.Symbol,
		Volume: row.Volume,
		Last:   row.Last,
		Bid:    row.Bid,
		Ask:    row.Ask,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !ready {
		return nil, nil
	}

	result, err := ticker.classifier.Classify(map[string]float64{
		"ignition":    output.Ignition,
		"compression": output.Compression,
		"trend":       output.Trend,
		"exhaustion":  output.Exhaustion,
		"strength":    output.Strength,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	measurement := logic.NewMeasurement(logic.SourcePumpDump, row.Symbol, row.Timestamp)
	measurement.AddMetric("rvol", output.RVOL)
	measurement.AddMetric("precursor", output.Precursor)
	measurement.AddMetric("spread", output.Spread)
	measurement.AddMetric("ignition", output.Ignition)
	measurement.AddMetric("compression", output.Compression)
	measurement.AddMetric("trend", output.Trend)
	measurement.AddMetric("exhaustion", output.Exhaustion)
	measurement.AddMetric("strength", output.Strength)

	if err := measurement.ApplyClassifier(
		result.Value,
		result.Confidence,
		result.EntryBaseline,
		result.ExitBaseline,
		result.Strength,
		result.Distribution,
	); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if err := measurement.Ready(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	return measurement, nil
}
