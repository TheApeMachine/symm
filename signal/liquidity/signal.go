package liquidity

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
)

/*
Liquidity is the Scarcity perspective, identifying opportunities in "thin"
markets by ranking a symbol's volume against the broader market.

1. What it measures exactly (in isolation)

The Liquidity signal identifies opportunities in thin markets by ranking
quote volume against peers.

Cross-Section Ranking: Ranks the dailyQuoteVol of all subscribed symbols.

Illiquidity Score: Specifically identifies symbols trading strictly below the
cross-section median of their peers.

Peak Scarcity: Uses a Peak gate to find symbols that are currently the most
illiquid in the universe.

---

2. Semantically, what story does it tell?

The Liquidity signal tells the story of convexity and market neglect.

The "Convexity" Story: It signals where a small amount of order flow will
cause the largest price displacement. It finds the "thinnest" pipes in the
exchange where price can move most easily.

The "Neglect" Story: It identifies assets that are being ignored by the
broader market, making them prime targets for sudden volatility once a trade
actually arrives.

1. Extreme Scarcity

The symbol is the most illiquid in the subscribed universe.
Indicators: Peak illiquidity rank with very low volume.
Semantic Meaning: High convexity/fragile — small flow, large displacement.

2. Median Depth

The symbol trades near the cross-section median.
Indicators: Middle rank with normal volume.
Semantic Meaning: Standard efficiency — typical market depth.

3. Robust Liquidity

The symbol ranks among the deepest markets.
Indicators: Bottom (deep) rank with high volume.
Semantic Meaning: Efficient/safe — thick, well-traded pipes.

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
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriteCloser
	tree        *dmt.Tree
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

	depth := equation.NewDepth()

	return &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			depth,
			probability.NewClassifier(
				datura.Acquire("liquidity-classifier", datura.APPJSON).Poke(
					[]string{"scarcityScore", "medianScore", "depthScore"},
					"inputs",
				),
			),
		),
	}
}

func (signal *Signal) Measure(query *datura.Artifact) *datura.Artifact {
	for stored := range signal.tree.Seek(query.Prefix()) {
		transport.Copy(query, stored)
		errnie.Error(transport.NewFlipFlop(query, signal.algo))
	}

	return query
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
