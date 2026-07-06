package sentiment

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	classifier *probability.ScoreClassifier
}

func NewTicker() *Ticker {
	return &Ticker{
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

func (ticker *Ticker) Measure(
	row kraken.TickerData,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	if crossSection == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "sentiment: cross-section required", nil,
		))
	}

	change := row.ChangePct / 100
	breadth := crossSection.Breadth()

	leaderStrength := 0.0
	leaderEvidence := 0.0
	relativeLead := 0.0

	if crossSection.IsLeader(row.Symbol, change) {
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

	result, err := ticker.classifier.Classify(map[string]float64{
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
		Symbol:        row.Symbol,
		At:            row.Timestamp,
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

	return []*types.Measurement{measurement}, nil
}
