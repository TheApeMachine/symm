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

# Summary of Liquidity Categories

| Category         | Rank vs. Peers   | Volume   | Market "Feel"                |
|:-----------------|:-----------------|:---------|:-----------------------------|
| Extreme Scarcity | Peak Illiquidity | Very Low | High Convexity / Fragile     |
| Median Depth     | Middle           | Normal   | Standard Efficiency          |
| Robust Liquidity | Bottom (Deep)    | High     | Efficient / Safe             |
*/
/*
Signal identifies opportunities in thin markets by ranking quote volume against peers.
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
			"liquidity", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"scarcityScore",
				"medianScore",
				"depthScore",
			},
			"categoryIndexes": []float64{
				float64(logic.CategoryIndex(logic.CategoryExtremeScarcity)),
				float64(logic.CategoryIndex(logic.CategoryMedianDepth)),
				float64(logic.CategoryIndex(logic.CategoryRobustLiquidity)),
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

			volume := datura.Peek[float64](datapoint, "data", rowIndex, "volume")
			peers := crossSection.Volumes()
			median := volume

			if len(peers) >= 2 {
				if value, ok := statistic.MedianOf(peers); ok {
					median = value
				}
			}

			if median <= 0 {
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
			measurement.Poke("output", "root")
			measurement.Poke([]string{
				"scarcityScore",
				"medianScore",
				"depthScore",
				"strength",
			}, "inputs")

			if err := signal.classifier.Apply(measurement); err != nil {
				if !yield(measurement.WithError(errnie.Error(err))) {
					return
				}

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
