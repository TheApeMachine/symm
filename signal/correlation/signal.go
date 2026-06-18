package correlation

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	marketsection "github.com/theapemachine/symm/market"
	feed "github.com/theapemachine/symm/signal"
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
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	tree         *dmt.Tree
	CrossSection *marketsection.CrossSection
}

/*
NewSignal composes the cohort pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, crossSectionErr := marketsection.NewCrossSection(&marketsection.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   16,
		MinBars:     4,
		BreadthHist: 16,
	})

	if crossSectionErr != nil {
		cancel()

		return nil
	}

	cohort := algorithm.NewCohort()

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		tree:         dmt.NewTree(""),
		CrossSection: crossSection,
		algo: nomagique.Number(
			cohort,
			probability.NewClassifier(
				cohort.HerdReading(),
				cohort.AlphaReading(),
				cohort.NoiseReading(),
				cohort.StressReading(),
			),
		),
	}
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "measurement":
		if artifact != nil {
			signal.Measure(*artifact)
		}
	}

	return nil
}

func (signal *Signal) Measure(query datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	feature := signal.featureArtifact(scope)

	if feature == nil {
		return nil
	}

	processed := datura.Acquire("correlation", datura.APPJSON)

	if processed == nil {
		feature.Release()
		return nil
	}

	payload, payloadOK := feature.PayloadQuiet()

	feature.Release()

	if !payloadOK {
		processed.Release()
		return nil
	}

	if processed.WithPayload(payload) == nil {
		processed.Release()
		return nil
	}

	if flipErr := transport.NewFlipFlop(processed, signal.algo); flipErr != nil {
		_ = processed.WithError(flipErr)
	}

	if datura.Peek[int](processed, "classifier.category") <= 0 {
		processed.Release()
		return nil
	}

	if datura.Peek[float64](processed, "classifier.confidence") <= 0 {
		processed.Release()
		return nil
	}

	processed.WithRole("measurement")
	processed.WithScope(scope)

	feed.InsertMeasurement(signal.tree, processed)

	return processed
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	window := signal.CrossSection.MinBarsRequired()
	at := time.Now()
	snapshot := signal.CrossSection.PeerWindowSnapshot(window, at)
	symbolReturns := signal.CrossSection.SymbolReturns(scope, window)

	if len(symbolReturns) < window || len(snapshot.MarketReturns) < window {
		return nil
	}

	peerCorrelations := snapshot.PeerCorrelations
	peerEnergies := snapshot.PeerEnergies

	samples := []float64{float64(window)}
	samples = append(samples,
		float64(len(symbolReturns)),
		float64(len(snapshot.MarketReturns)),
		float64(len(peerCorrelations)),
		float64(len(peerEnergies)),
	)
	samples = append(samples, symbolReturns...)
	samples = append(samples, snapshot.MarketReturns...)
	samples = append(samples, peerCorrelations...)
	samples = append(samples, peerEnergies...)

	payload, err := json.Marshal(samples)

	if err != nil {
		return nil
	}

	artifact := datura.Acquire("cohort-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	return artifact
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
