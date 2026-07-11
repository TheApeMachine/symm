package pumpdump

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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
				float64(types.CategoryIndex(types.CategoryVerticalIgnition)),
				float64(types.CategoryIndex(types.CategoryCoiledCompression)),
				float64(types.CategoryIndex(types.CategoryOrganicTrend)),
				float64(types.CategoryIndex(types.CategoryFadedExhaustion)),
			},
		),
	}
}

func (ticker *Ticker) Measure(
	row kraken.TickerData,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	if row.Symbol == "" || row.Volume <= 0 || row.Last == nil || row.Last.Sign() <= 0 ||
		row.Bid == nil || row.Bid.Sign() <= 0 ||
		row.Ask == nil || row.Ask.Sign() <= 0 {
		return nil, nil
	}

	output, ready, maturity, err := ticker.ignition.Measure(equation.IgnitionInput{
		Symbol: row.Symbol,
		Volume: row.Volume,
		Last:   row.Last.Float64(),
		Bid:    row.Bid.Float64(),
		Ask:    row.Ask.Float64(),
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

	categories := []types.CategoryType{
		types.VerticalIgnition,
		types.CoiledCompression,
		types.OrganicTrend,
		types.FadedExhaustion,
	}
	strengths := []float64{
		output.Ignition,
		output.Compression,
		output.Trend,
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
		Source:        types.SourcePumpDump,
		Stream:        "ticker",
		Symbol:        row.Symbol,
		At:            row.Timestamp,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Maturity:      maturity,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"rvol":        output.RVOL,
			"precursor":   output.Precursor,
			"spread":      output.Spread,
			"ignition":    output.Ignition,
			"compression": output.Compression,
			"trend":       output.Trend,
			"exhaustion":  output.Exhaustion,
			"strength":    output.Strength,
		},
	}

	return []*types.Measurement{measurement}, nil
}
