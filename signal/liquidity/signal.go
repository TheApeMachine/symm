package liquidity

import (
	"context"
	"iter"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
	"github.com/theapemachine/symm/statutil"
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
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
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
			row, rowErr := market.SymbolFromTicker(datapoint, rowIndex)

			if rowErr != nil {
				return
			}

			peers := crossSection.Volumes()

			median := row.Volume

			if len(peers) >= 2 {
				median = statutil.Median(peers)
			}

			if median <= 0 {
				continue
			}

			relative := row.Volume / median
			scarcity := math.Max(0, 1-relative)
			depth := math.Max(0, relative-1)
			balance := 1 / (1 + math.Abs(relative-1))

			shares := []dist.Share{
				{Key: "scarcityScore", Category: logic.CategoryExtremeScarcity, Mass: scarcity},
				{Key: "medianScore", Category: logic.CategoryMedianDepth, Mass: balance},
				{Key: "depthScore", Category: logic.CategoryRobustLiquidity, Mass: depth},
			}

			measurement := datura.Acquire("liquidity", datura.APPJSON)
			measurement.WithRole("measurement")
			measurement.WithScope(row.Name)
			errnie.Error(measurement.SetOrigin(string(logic.SourceLiquidity)))
			measurement.SetTimestamp(datapoint.Timestamp())

			measurement.MergeOutput("relativeVolume", relative)
			confidence := dist.Write(measurement, shares)

			if confidence <= 0 {
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
