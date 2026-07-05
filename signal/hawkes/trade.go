package hawkes

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Trade struct {
	sample     *algorithm.TradeExcitationSample
	excitation *algorithm.Excitation
	classifier *probability.ScoreClassifier
}

func NewTrade() *Trade {
	return &Trade{
		sample:     algorithm.NewTradeExcitationSample(),
		excitation: algorithm.NewExcitation(),
		classifier: probability.NewScoreClassifier(
			[]string{"frenzy", "saturation", "organic", "exhaustion"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryFrenzy)),
				float64(logic.CategoryIndex(logic.CategorySaturation)),
				float64(logic.CategoryIndex(logic.CategoryOrganic)),
				float64(logic.CategoryIndex(logic.CategoryExhaustion)),
			},
		),
	}
}

func (trade *Trade) Measure(row kraken.TradeData) (*logic.Measurement, error) {
	input, ready, err := trade.sample.MeasureTrade(algorithm.TradeExcitationInput{
		Symbol:   row.Symbol,
		Side:     row.Side,
		UnixNano: row.Timestamp.UnixNano(),
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, ready, err := trade.excitation.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !ready || output.Strength <= 0 {
		return nil, nil
	}

	result, err := trade.classifier.Classify(map[string]float64{
		"frenzy":     output.Frenzy,
		"saturation": output.Saturation,
		"organic":    output.Organic,
		"exhaustion": output.Exhaustion,
		"strength":   output.Strength,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	measurement := logic.NewMeasurement(logic.SourceHawkes, row.Symbol, row.Timestamp)
	measurement.AddMetric("frenzy", output.Frenzy)
	measurement.AddMetric("saturation", output.Saturation)
	measurement.AddMetric("organic", output.Organic)
	measurement.AddMetric("exhaustion", output.Exhaustion)
	measurement.AddMetric("strength", output.Strength)
	measurement.AddMetric("branchingRatio", output.BranchingRatio)
	measurement.AddMetric("spectralRadius", output.SpectralRadius)
	measurement.AddMetric("stationarityMargin", output.StationarityMargin)
	measurement.AddMetric("baselineMu", output.BaselineMu)
	measurement.AddMetric("intensityRatio", output.IntensityRatio)

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
