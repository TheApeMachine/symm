package liquidity

import (
	"context"
	"io"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	marketsection "github.com/theapemachine/symm/market"
	"gonum.org/v1/gonum/stat"
)

/*
Liquidity is the Scarcity perspective, identifying opportunities in thin markets
by ranking a symbol's volume against the broader market.

1. What it measures exactly (in isolation)

The Liquidity signal identifies opportunities in thin markets by ranking a
symbol's volume against the broader market.

Cross-Section Ranking: Ranks latest ticker volume snapshots across all
subscribed symbols (not an explicit daily rollup window).

Illiquidity Score: Identifies symbols trading strictly below the cross-section
median of their peers.

Peak Scarcity: Flags symbols at the universe minimum volume (isPeakScarcity),
not a separate peak-detector primitive.

---

2. Semantically, what story does it tell?

The "Convexity" Story: It signals where a small amount of order flow will cause
the largest price displacement. It finds the thinnest pipes in the exchange where
price can move most easily.

The "Neglect" Story: It identifies assets that are being ignored by the broader
market, making them prime targets for sudden volatility once a trade actually
arrives.

1. Extreme Scarcity

The symbol is at peak illiquidity versus peers.
Indicators: Peak illiquidity rank with very low volume.
Semantic Meaning: High convexity / fragile — small flow moves price sharply.

2. Median Depth

Volume sits near the cross-section middle band.
Indicators: Middle rank with normal peer-relative volume.
Semantic Meaning: Standard efficiency — typical displacement per unit flow.

3. Robust Liquidity

Volume ranks deep versus peers.
Indicators: Bottom rank (deepest book) with high volume.
Semantic Meaning: Efficient / safe — thick pipe absorbs flow without sharp moves.

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
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriteCloser
	tree         *dmt.Tree
	CrossSection *marketsection.CrossSection
}

/*
NewSignal composes the depth pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, err := marketsection.NewCrossSection(&marketsection.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   16,
		MinBars:     4,
		BreadthHist: 16,
	})

	if err != nil {
		cancel()

		return nil
	}

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		tree:         tree,
		CrossSection: crossSection,
		algo: nomagique.Number(
			equation.NewDepth(datura.Acquire("liquidity-depth", datura.APPJSON)),
			probability.NewClassifier(
				datura.Acquire("liquidity-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"scarcityScore", "medianScore", "depthScore"},
				}),
			),
		),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil || signal.CrossSection == nil {
		return nil
	}

	row, rowErr := marketsection.SymbolFromTicker(datapoint)

	if rowErr != nil {
		return nil
	}

	if errnie.Error(signal.CrossSection.Observe(row)) != nil {
		return nil
	}

	peers := signal.CrossSection.Volumes()

	if len(peers) < 2 {
		return nil
	}

	features := depthFeatureBatch(row.Volume, peers)
	stored := datura.Acquire("liquidity-depth", datura.APPJSON)
	stored.WithPayload(equation.MarshalFeaturesPayload(features))

	if errnie.Error(transport.NewFlipFlop(
		stored, signal.algo,
	)) != nil {
		stored.Release()

		return nil
	}

	confidence := datura.Peek[float64](stored, "output", "confidence")
	uniformConfidence := 1.0 / 3.0

	if confidence <= 0 ||
		math.IsNaN(confidence) ||
		math.IsInf(confidence, 0) ||
		confidence <= uniformConfidence+1e-12 {
		stored.Release()

		return nil
	}

	return stored
}

func depthFeatureBatch(quoteVolume float64, peers []float64) []float64 {
	peerCount := len(peers)
	relativeVolume := 0.0
	baselineReady := 0.0
	sortedPeers := append([]float64(nil), peers...)
	sort.Float64s(sortedPeers)
	median := stat.Quantile(0.5, stat.LinInterp, sortedPeers, nil)

	if median > 0 {
		relativeVolume = quoteVolume / median
		baselineReady = 1
	}

	batch := make([]float64, 0, 2+peerCount+2)
	batch = append(batch, quoteVolume, float64(peerCount))
	batch = append(batch, peers...)
	batch = append(batch, relativeVolume, baselineReady)

	return batch
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
