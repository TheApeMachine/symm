package correlation

import (
	"context"
	"io"
	"math"
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

Synchronized Log-Returns: Aligns price windows onto a shared 10-second bar
grid and correlates each symbol's return stream with the cross-section median
(not all pairwise correlations).

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
Indicators: High correlation with high variance (peer-adaptive quantile gates,
not a fixed 0.85 threshold).
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

| Category          | Correlation Level      | Variance | Market "Feel"                           |
|:------------------|:-----------------------|:---------|:----------------------------------------|
| Systemic Herd     | High (adaptive quantile)| High     | Global Beta / Momentum Drift            |
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
	algo            io.ReadWriteCloser
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
		MatchWindow: 10 * time.Second,
		ReturnCap:   16,
		MinBars:     4,
		BreadthHist: 16,
	}

	crossSection, err := marketsection.NewCrossSection(&cfg)

	if err != nil {
		cancel()
		return nil
	}

	return &Signal{
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		subscribers:     &sync.Map{},
		tree:            tree,
		crossSectionCfg: cfg,
		CrossSection:    crossSection,
		algo: nomagique.Number(
			equation.NewCohort(datura.Acquire("correlation-cohort", datura.APPJSON)), probability.NewClassifier(
				datura.Acquire("correlation-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"herdScore", "alphaScore", "noiseScore", "stressScore"},
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

	window := signal.CrossSection.MinBarsRequired()
	symbolReturns := signal.CrossSection.SymbolReturns(row.Name, window)
	snapshot := signal.CrossSection.PeerWindowSnapshot(window, row.Updated)

	if len(symbolReturns) < window ||
		len(snapshot.MarketReturns) < window ||
		len(snapshot.PeerCorrelations) < 2 {
		return nil
	}

	features := cohortFeatureBatch(
		window,
		signal.crossSectionCfg.MatchWindow.Seconds(),
		symbolReturns,
		snapshot.MarketReturns,
		snapshot.PeerCorrelations,
		snapshot.PeerEnergies,
	)

	stored := datura.Acquire("correlation-cohort", datura.APPJSON)
	stored.WithPayload(equation.MarshalFeatureSchema(equation.CohortInputKeys, features))

	if errnie.Error(transport.NewFlipFlop(
		stored, signal.algo,
	)) != nil {
		stored.Release()

		return nil
	}

	confidence := datura.Peek[float64](stored, "output", "confidence")
	uniformConfidence := 1.0 / 4.0

	if confidence <= 0 ||
		math.IsNaN(confidence) ||
		math.IsInf(confidence, 0) ||
		confidence <= uniformConfidence+1e-12 {
		stored.Release()

		return nil
	}

	return stored
}

func cohortFeatureBatch(
	window int,
	barSpacingSeconds float64,
	symbolReturns, marketReturns, peerCorrelations, peerEnergies []float64,
) []float64 {
	batch := []float64{float64(window)}
	series := [][]float64{
		symbolReturns,
		marketReturns,
		peerCorrelations,
		peerEnergies,
	}

	for _, segment := range series {
		batch = append(batch, float64(len(segment)))
	}

	batch = append(batch, barSpacingSeconds)

	for _, segment := range series {
		batch = append(batch, segment...)
	}

	return batch
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
