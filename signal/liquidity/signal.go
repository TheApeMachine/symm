package liquidity

import (
	"context"
	"iter"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
Liquidity is the Scarcity perspective, identifying opportunities in thin markets
by ranking a symbol's volume against the broader market.
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	tree       *dmt.Tree
	classifier *probability.ScoreClassifier
}

func NewSignal(ctx context.Context, tree *dmt.Tree) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		classifier: probability.NewScoreClassifier(
			[]string{"scarcityScore", "medianScore", "depthScore"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryExtremeScarcity)),
				float64(logic.CategoryIndex(logic.CategoryMedianDepth)),
				float64(logic.CategoryIndex(logic.CategoryRobustLiquidity)),
			},
		),
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
		if crossSection == nil {
			yield(datapoint.WithError(errnie.Error(errnie.Err(
				errnie.Validation,
				"liquidity: cross-section required",
				nil,
			))))
			return
		}

		role := datura.Peek[string](datapoint, "role")
		if role != "ticker" {
			return
		}

		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")
			if symbol == "" {
				return
			}

			volume := datura.Peek[float64](datapoint, "data", rowIndex, "volume")
			peers := crossSection.Volumes()
			if len(peers) < 2 {
				continue
			}

			median, ok := statistic.MedianOf(peers)
			if !ok || median <= 0 {
				continue
			}

			relative := volume / median
			scarcity := math.Max(0, 1-relative)
			depth := math.Max(0, relative-1)
			balance := 1 / (1 + math.Abs(relative-1))
			strength := max(scarcity, max(balance, depth))

			measurement := datura.Acquire("liquidity", datura.APPJSON)
			measurement.WithRole("measurement")
			measurement.WithScope(symbol)
			errnie.Error(measurement.SetOrigin(string(logic.SourceLiquidity)))
			measurement.SetTimestamp(datapoint.Timestamp())
			measurement.MergeOutputs(map[string]any{
				"relativeVolume": relative,
				"scarcityScore":  scarcity,
				"medianScore":    balance,
				"depthScore":     depth,
				"strength":       strength,
			})

			result, err := signal.classifier.Classify(map[string]float64{
				"scarcityScore": scarcity,
				"medianScore":   balance,
				"depthScore":    depth,
				"strength":      strength,
			})
			if err != nil {
				if !yield(measurement.WithError(errnie.Error(err))) {
					return
				}

				continue
			}

			for key, value := range result.Outputs() {
				measurement.MergeOutput(key, value)
			}

			if datura.Peek[float64](measurement, "output", "confidence") <= 0 {
				measurement.Release()
				continue
			}

			measurement.Poke("output", "root")

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.err
}
