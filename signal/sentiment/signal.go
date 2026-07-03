package sentiment

import (
	"context"
	"iter"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
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
	tree       *dmt.Tree
	classifier *probability.Classifier
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		classifier: probability.NewClassifier(datura.Acquire(
			"sentiment", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"surgeScore",
				"divergentScore",
				"slumpScore",
			},
			"categoryIndexes": []float64{
				float64(logic.CategoryIndex(logic.CategoryRiskOnSurge)),
				float64(logic.CategoryIndex(logic.CategoryDivergentMove)),
				float64(logic.CategoryIndex(logic.CategorySystemicSlump)),
			},
		})),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil || crossSection == nil {
			return
		}

		if datura.Peek[string](datapoint, "channel") != "ticker" {
			return
		}

		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")

			if symbol == "" {
				return
			}

			change := datura.Peek[float64](datapoint, "data", rowIndex, "change_pct") / 100
			breadth := crossSection.Breadth()

			leaderStrength := 0.0
			leaderEvidence := 0.0
			relativeLead := 0.0

			if crossSection.IsLeader(symbol, change) {
				leaderStrength = math.Abs(change)
				threshold := crossSection.LeadershipThreshold()

				if threshold > 0 {
					leaderEvidence = leaderStrength / threshold
				} else if leaderStrength > 0 {
					leaderEvidence = 1
				}

				relativeLead = 1
			}

			leaderMass := leaderEvidence / (1 + leaderEvidence)
			surgeScore := breadth * leaderEvidence * math.Max(relativeLead, 1/(1+leaderEvidence))
			divergentScore := (1 - breadth) * relativeLead * leaderEvidence
			slumpScore := (1 - breadth) * (1 - relativeLead) / (1 + leaderMass)
			strength := max(surgeScore, max(divergentScore, slumpScore))

			measurement := datura.Acquire("sentiment", datura.APPJSON)
			measurement.WithRole("measurement")
			measurement.WithScope(symbol)
			errnie.Error(measurement.SetOrigin(string(logic.SourceSentiment)))
			measurement.SetTimestamp(datapoint.Timestamp())

			measurement.MergeOutput("breadth", breadth)
			measurement.MergeOutput("leaderStrength", leaderStrength)
			measurement.MergeOutput("leaderEvidence", leaderEvidence)
			measurement.MergeOutputs(map[string]any{
				"surgeScore":     surgeScore,
				"divergentScore": divergentScore,
				"slumpScore":     slumpScore,
				"strength":       strength,
			})
			measurement.Poke("output", "root")
			measurement.Poke([]string{
				"surgeScore",
				"divergentScore",
				"slumpScore",
				"strength",
			}, "inputs")

			if err := signal.classifier.Apply(measurement); err != nil {
				if !yield(measurement.WithError(errnie.Error(err))) {
					return
				}

				continue
			}

			if datura.Peek[float64](measurement, "output", "confidence") <= 0 {
				measurement.Release()
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
