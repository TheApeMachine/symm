package pumpdump

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Trade struct {
	sample     *algorithm.TradeFlowSample
	flow       *equation.Flow
	classifier *probability.ScoreClassifier
}

func NewTrade() *Trade {
	return &Trade{
		sample: algorithm.NewTradeFlowSample(),
		flow:   equation.NewFlow(),
		classifier: probability.NewScoreClassifier(
			[]string{"absorption", "drive", "balance", "starvation"},
			[]float64{
				float64(types.CategoryIndex(types.CategoryHiddenAbsorption)),
				float64(types.CategoryIndex(types.CategoryAggressiveDrive)),
				float64(types.CategoryIndex(types.CategoryStochasticBalance)),
				float64(types.CategoryIndex(types.CategoryVolumeStarvation)),
			},
		),
	}
}

func (trade *Trade) Measure(row kraken.TradeData) ([]*types.Measurement, error) {
	input, ready, err := trade.sample.Measure(algorithm.TradeFlowInput{
		Symbol:   row.Symbol,
		Price:    row.Price.Float64(),
		Quantity: row.Qty,
		Side:     row.Side,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := trade.flow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if output.Value <= 0 {
		return nil, nil
	}

	result, err := trade.classifier.Classify(map[string]float64{
		"absorption": output.Absorption,
		"drive":      output.Drive,
		"balance":    output.Balance,
		"starvation": output.Starvation,
		"strength":   output.Value,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	categories := []types.CategoryType{
		types.HiddenAbsorption,
		types.AggressiveDrive,
		types.StochasticBalance,
		types.VolumeStarvation,
	}
	strengths := []float64{
		output.Absorption,
		output.Drive,
		output.Balance,
		output.Starvation,
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
		Source:        types.SourcePumpDump,
		Symbol:        row.Symbol,
		At:            row.Timestamp,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"absorption": output.Absorption,
			"drive":      output.Drive,
			"balance":    output.Balance,
			"starvation": output.Starvation,
			"strength":   output.Value,
		},
	}

	return []*types.Measurement{measurement}, nil
}
