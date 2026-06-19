package correlation

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	marketsection "github.com/theapemachine/symm/market"
)

/*
Correlation is the "Herd Behavior" perspective, measuring synchronized return
correlation across the subscribed universe using a rolling window of
log-returns.

1. What it measures exactly (in isolation)

The Correlation signal measures synchronized return correlation across the
subscribed universe using a rolling window of log-returns. It determines if
the market is moving as a single, indistinguishable block or if individual
assets are exhibiting unique behavior.

Synchronized Log-Returns: It aligns price windows onto a shared time grid
(e.g., 10-second bars) to calculate the Pearson correlation between pairs.

Peak Score: It identifies symbols that are hitting a "peak" in their
correlation to the broader market, using an adaptive peak gate.

Hayashi-Yoshida Fallback: For high-frequency, asynchronous data where trades
don't align perfectly on time bars, it uses the H-Y estimator to capture
overlapping return intervals.

---

2. Semantically, what story does it tell?

The Correlation signal tells the story of herd behavior and systemic coupling.

The "Rising Tide" Story: It asks: "Is this asset special, or is it just being
dragged along by the herd?". High correlation indicates that macro-systemic
forces are dominant.

The "De-coupling" Story: It identifies "alpha" opportunities by spotting when
an asset stops following its peers, suggesting a local catalyst is at play.

The "Liquidation" Story: Sudden spikes in cross-asset correlation toward 1.0
often signal systemic panics or liquidation cascades where everything is sold
at once.

1. Systemic Herd

The asset is moving in lockstep with the broader market.
Indicators: High correlation (> 0.85) with high variance.
Semantic Meaning: Global beta/momentum drift — macro forces dominate.

2. Decoupled Alpha

The asset is moving independently with high energy.
Indicators: Low correlation with high variance.
Semantic Meaning: Unique driver/leading move — idiosyncratic alpha.

3. Stochastic Noise

Low energy with no clear coupling.
Indicators: Low correlation with low variance.
Semantic Meaning: Quiet/indecisive — no herd and no catalyst.

4. Divergent Stress

The asset moves against the herd under stress.
Indicators: Negative correlation with high variance.
Semantic Meaning: Contrarian move/relative weakness — counter-herd stress.

# Summary of Correlation Categories

| Category          | Correlation Level | Variance | Market "Feel"                           |
|:------------------|:------------------|:---------|:----------------------------------------|
| Systemic Herd     | High (> 0.85)     | High     | Global Beta / Momentum Drift            |
| Decoupled Alpha   | Low               | High     | Unique Driver / Leading Move            |
| Stochastic Noise  | Low               | Low      | Quiet / Indecisive                      |
| Divergent Stress  | Negative          | High     | Contrarian Move / Relative Weakness     |
*/
/*
Signal measures how each symbol's return stream correlates with the cross-section median.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx             context.Context
	cancel          context.CancelFunc
	err             error
	pool            *qpool.Q[any]
	subscribers     *sync.Map
	algo            io.ReadWriter
	tree            *dmt.Tree
	crossSectionCfg marketsection.CrossSectionConfig
	CrossSection    *marketsection.CrossSection
}

/*
NewSignal composes the cohort pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	cfg := marketsection.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   16,
		MinBars:     4,
		BreadthHist: 16,
	}

	crossSection, crossSectionErr := marketsection.NewCrossSection(&cfg)

	if crossSectionErr != nil {
		cancel()

		return nil
	}

	cohort := algorithm.NewCohort()

	return &Signal{
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		subscribers:     &sync.Map{},
		tree:            tree,
		crossSectionCfg: cfg,
		CrossSection:    crossSection,
		algo: nomagique.Number(
			cohort,
			probability.NewClassifier(
				datura.Acquire("correlation-classifier", datura.APPJSON).Poke(
					[]string{"herdScore", "alphaScore", "noiseScore", "stressScore"},
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

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
