package sentiment

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
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
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	classifier *probability.ScoreClassifier
}

func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		classifier: probability.NewScoreClassifier(
			[]string{"surgeScore", "divergentScore", "slumpScore"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryRiskOnSurge)),
				float64(logic.CategoryIndex(logic.CategoryDivergentMove)),
				float64(logic.CategoryIndex(logic.CategorySystemicSlump)),
			},
		),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(
	input market.Input,
	crossSection *market.CrossSection,
) ([]*logic.Measurement, error) {
	if crossSection == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "sentiment: cross-section required", nil,
		))
	}

	if input.Role != "ticker" {
		return nil, nil
	}

	measurements := make([]*logic.Measurement, 0, len(input.Ticker))

	for _, ticker := range input.Ticker {
		measurement, err := signal.measure(ticker, crossSection)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))
		}

		if measurement == nil {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

func (signal *Signal) measure(
	ticker kraken.TickerData,
	crossSection *market.CrossSection,
) (*logic.Measurement, error) {
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

	measurement := logic.NewMeasurement(logic.SourceSentiment, ticker.Symbol, ticker.Timestamp)
	measurement.AddMetric("breadth", breadth)
	measurement.AddMetric("leaderStrength", leaderStrength)
	measurement.AddMetric("leaderEvidence", leaderEvidence)
	measurement.AddMetric("surgeScore", surgeScore)
	measurement.AddMetric("divergentScore", divergentScore)
	measurement.AddMetric("slumpScore", slumpScore)
	measurement.AddMetric("strength", strength)

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

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
