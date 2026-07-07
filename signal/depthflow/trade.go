package depthflow

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Trade struct {
	sample     *algorithm.BookflowSample
	bookflow   *equation.Bookflow
	classifier *probability.ScoreClassifier
}

func NewTrade(
	sample *algorithm.BookflowSample,
	bookflow *equation.Bookflow,
	classifier *probability.ScoreClassifier,
) *Trade {
	return &Trade{
		sample:     sample,
		bookflow:   bookflow,
		classifier: classifier,
	}
}

func (trade *Trade) Measure(row kraken.TradeData) ([]*types.Measurement, error) {
	input, ready, err := trade.sample.MeasureTrade(algorithm.BookflowTradeInput{
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

	output, err := trade.bookflow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !output.Ready || output.Strength <= 0 {
		return nil, nil
	}

	result, err := trade.classifier.Classify(map[string]float64{
		"loadedScore":  output.LoadedScore,
		"spoofScore":   output.SpoofScore,
		"thinScore":    output.ThinScore,
		"neutralScore": output.NeutralScore,
		"strength":     output.Strength,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	categories := []types.CategoryType{
		types.LoadedImbalance,
		types.SpoofTrap,
		types.BookThinning,
		types.DenseNeutrality,
	}
	strengths := []float64{
		output.LoadedScore,
		output.SpoofScore,
		output.ThinScore,
		output.NeutralScore,
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
		Source:        types.SourceDepthFlow,
		Symbol:        row.Symbol,
		At:            row.Timestamp,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"loadedScore":  output.LoadedScore,
			"spoofScore":   output.SpoofScore,
			"thinScore":    output.ThinScore,
			"neutralScore": output.NeutralScore,
			"strength":     output.Strength,
		},
	}

	return []*types.Measurement{measurement}, nil
}
