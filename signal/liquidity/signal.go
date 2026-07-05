package liquidity

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Liquidity is the Scarcity perspective, identifying opportunities in thin markets
by ranking a symbol's volume against the broader types.
*/
type Signal[T any] struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	classifier *probability.ScoreClassifier
}

func NewSignal[T any](ctx context.Context) *Signal[T] {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal[T]{
		ctx:    ctx,
		cancel: cancel,
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

func (signal *Signal[T]) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal[T]) Categories() []types.CategoryType {
	return []types.CategoryType{
		types.ExtremeScarcity,
		types.MedianDepth,
		types.RobustLiquidity,
	}
}

func (signal *Signal[T]) Measure(
	input T,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	switch row := any(input).(type) {
	case kraken.TickerData:
		if crossSection == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, "liquidity: cross-section required", nil,
			))
		}

		measurement, err := signal.measure(row, crossSection)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))
		}

		if measurement == nil {
			return nil, nil
		}

		return []*types.Measurement{measurement}, nil
	}

	return nil, nil
}

func (signal *Signal[T]) measure(
	ticker kraken.TickerData,
	crossSection *types.CrossSection,
) (*types.Measurement, error) {
	peers := crossSection.Volumes()

	if len(peers) < 2 {
		return nil, nil
	}

	median, ok := statistic.MedianOf(peers)

	if !ok || median <= 0 {
		return nil, nil
	}

	relative := ticker.Volume / median
	scarcity := math.Max(0, 1-relative)
	depth := math.Max(0, relative-1)
	balance := 1 / (1 + math.Abs(relative-1))
	strength := max(scarcity, max(balance, depth))

	result, err := signal.classifier.Classify(map[string]float64{
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
		Symbol:        ticker.Symbol,
		At:            ticker.Timestamp,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"relativeVolume": relative,
			"scarcityScore":  scarcity,
			"medianScore":    balance,
			"depthScore":     depth,
			"strength":       strength,
		},
	}

	return measurement, nil
}

func (signal *Signal[T]) Error() error {
	return signal.err
}

func (signal *Signal[T]) Close() error {
	signal.cancel()
	return signal.err
}
