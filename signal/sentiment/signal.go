package sentiment

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Sentiment is the Bullish Breadth perspective, measuring global market conviction
by looking at the behavior of the entire universe simultaneously.

# Summary of Sentiment Categories

| Category       | Breadth | Leader Strength | Market "Feel"           |
|:---------------|:--------|:----------------|:------------------------|
| Risk-On Surge  | High    | Strong          | Rising Tide / Global Buy|
| Divergent Move | Low     | Strong          | Idiosyncratic Alpha     |
| Systemic Slump | Low     | Weak            | Global Risk-Off         |
*/
/*
Signal measures global market conviction from breadth and leadership performance.
See the struct comment block for category semantics.
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
			[]string{"surgeScore", "divergentScore", "slumpScore"},
			[]float64{
				float64(types.CategoryIndex(types.CategoryRiskOnSurge)),
				float64(types.CategoryIndex(types.CategoryDivergentMove)),
				float64(types.CategoryIndex(types.CategorySystemicSlump)),
			},
		),
	}
}

func (signal *Signal[T]) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal[T]) Categories() []types.CategoryType {
	return []types.CategoryType{
		types.RiskOnSurge,
		types.DivergentMove,
		types.SystemicSlump,
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
				errnie.Validation, "sentiment: cross-section required", nil,
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
	change := ticker.ChangePct / 100
	breadth := crossSection.Breadth()

	leaderStrength := 0.0
	leaderEvidence := 0.0
	relativeLead := 0.0

	if crossSection.IsLeader(ticker.Symbol, change) {
		leaderStrength = math.Abs(change)
		threshold := crossSection.LeadershipThreshold()

		if threshold <= 0 {
			return nil, nil
		}

		leaderEvidence = leaderStrength / threshold
		relativeLead = 1
	}

	leaderMass := leaderEvidence / (1 + leaderEvidence)
	surgeScore := breadth * leaderEvidence * math.Max(relativeLead, 1/(1+leaderEvidence))
	divergentScore := (1 - breadth) * relativeLead * leaderEvidence
	slumpScore := (1 - breadth) * (1 - relativeLead) / (1 + leaderMass)
	strength := max(surgeScore, max(divergentScore, slumpScore))

	if strength <= 0 {
		return nil, nil
	}

	result, err := signal.classifier.Classify(map[string]float64{
		"surgeScore":     surgeScore,
		"divergentScore": divergentScore,
		"slumpScore":     slumpScore,
		"strength":       strength,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	categories := []types.CategoryType{
		types.RiskOnSurge,
		types.DivergentMove,
		types.SystemicSlump,
	}
	strengths := []float64{
		surgeScore,
		divergentScore,
		slumpScore,
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
		Source:        types.SourceSentiment,
		Symbol:        ticker.Symbol,
		At:            ticker.Timestamp,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"breadth":        breadth,
			"leaderStrength": leaderStrength,
			"leaderEvidence": leaderEvidence,
			"surgeScore":     surgeScore,
			"divergentScore": divergentScore,
			"slumpScore":     slumpScore,
			"strength":       strength,
		},
	}

	return measurement, nil
}

func (signal *Signal[T]) Error() error {
	return signal.err
}

func (signal *Signal[T]) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
