package liquidity

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	classifier *probability.ScoreClassifier
}

func NewTicker() *Ticker {
	return &Ticker{
		classifier: probability.NewScoreClassifier(
			[]string{"scarcityScore", "medianScore", "depthScore"},
			[]float64{
				float64(types.CategoryIndex(types.CategoryExtremeScarcity)),
				float64(types.CategoryIndex(types.CategoryMedianDepth)),
				float64(types.CategoryIndex(types.CategoryRobustLiquidity)),
			},
		),
	}
}

func (ticker *Ticker) Measure(
	row kraken.TickerData,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	if crossSection == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "liquidity: cross-section required", nil,
		))
	}

	notionalPeers := crossSection.QuoteNotionals()
	depthPeers := crossSection.ExecutableDepths()

	if len(notionalPeers) < 2 || len(depthPeers) < 2 {
		return nil, nil
	}

	notionalMedian, ok := statistic.MedianOf(notionalPeers)

	if !ok || notionalMedian <= 0 {
		return nil, nil
	}

	depthMedian, ok := statistic.MedianOf(depthPeers)

	if !ok || depthMedian <= 0 {
		return nil, nil
	}

	notional := types.QuoteNotional(row)
	executableDepth := types.ExecutableDepth(row)

	if notional <= 0 || executableDepth <= 0 {
		return nil, nil
	}

	// Geometric mean of the two ratios, not an arithmetic mean: it is the
	// scale-symmetric combinator for ratios (a 2x-notional/0.5x-depth
	// symbol nets to the peer median, not to a false 1.25x), so no side
	// dominates the other through a hand-picked weight.
	relative := math.Sqrt((notional / notionalMedian) * (executableDepth / depthMedian))
	scarcity := math.Max(0, 1-relative)
	depth := math.Max(0, relative-1)
	balance := 1 / (1 + math.Abs(relative-1))
	strength := max(scarcity, max(balance, depth))

	result, err := ticker.classifier.Classify(map[string]float64{
		"scarcityScore": scarcity,
		"medianScore":   balance,
		"depthScore":    depth,
		"strength":      strength,
	})

	if err != nil {
		return nil, err
	}

	categories := []types.CategoryType{
		types.ExtremeScarcity,
		types.MedianDepth,
		types.RobustLiquidity,
	}
	strengths := []float64{
		scarcity,
		balance,
		depth,
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
		Source:        types.SourceLiquidity,
		Stream:        "ticker",
		Symbol:        row.Symbol,
		At:            row.Timestamp,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"rvol":                  relative,
			"scarcityScore":         scarcity,
			"medianScore":           balance,
			"depthScore":            depth,
			"strength":              strength,
			"quoteNotional":         notional,
			"quoteNotionalMedian":   notionalMedian,
			"executableDepth":       executableDepth,
			"executableDepthMedian": depthMedian,
		},
	}

	return []*types.Measurement{measurement}, nil
}
