package hawkes

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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
				float64(types.CategoryIndex(types.CategoryFrenzy)),
				float64(types.CategoryIndex(types.CategorySaturation)),
				float64(types.CategoryIndex(types.CategoryOrganic)),
				float64(types.CategoryIndex(types.CategoryExhaustion)),
			},
		),
	}
}

func (trade *Trade) Measure(row kraken.TradeData) ([]*types.Measurement, error) {
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

	if !ready {
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

	categories := []types.CategoryType{
		types.Frenzy,
		types.Saturation,
		types.Organic,
		types.Exhaustion,
	}
	strengths := []float64{
		output.Frenzy,
		output.Saturation,
		output.Organic,
		output.Exhaustion,
	}
	categoryRows := make([]types.Category, 0, len(categories))

	for index, category := range categories {
		confidence := 0.0

		if index < len(result.Probabilities) {
			confidence = result.Probabilities[index]
		}

		categoryRows = append(categoryRows, types.Category{
			Type:       category,
			Confidence: confidence,
			Strength:   strengths[index],
		})
	}

	measurement := &types.Measurement{
		Source:        types.SourceHawkes,
		Stream:        "trades",
		Symbol:        row.Symbol,
		At:            row.Timestamp,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Maturity:      output.Maturity,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"frenzy":             output.Frenzy,
			"saturation":         output.Saturation,
			"organic":            output.Organic,
			"exhaustion":         output.Exhaustion,
			"strength":           output.Strength,
			"branchingRatio":     output.BranchingRatio,
			"spectralRadius":     output.SpectralRadius,
			"stationarityMargin": output.StationarityMargin,
			"baselineMu":         output.BaselineMu,
			"intensityRatio":     output.IntensityRatio,
		},
	}

	return []*types.Measurement{measurement}, nil
}
