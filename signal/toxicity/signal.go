package toxicity

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
Toxicity is the Quality perspective, analyzing the "honesty" of the book by
tracking how makers behave when a trade approaches.

1. What it measures exactly (in isolation)

The Toxicity signal analyzes the "honesty" of the book by tracking how makers
behave when a trade approaches.

Cancel-to-Fill Asymmetry: Measures the ratio of liquidity being "pulled"
(cancelled) versus liquidity being "hit" (filled).

Toxic Level Detection: Flags large, young, near-touch blocks that disappear
rather than fill — this is the signature of a bluff.

Directional Vacuum Inference: Bid vs ask cancel/fill ratios infer which side
is retreating internally; no separate directional output field is emitted.

---

2. Semantically, what story does it tell?

The Toxicity signal tells the story of sincere versus fake liquidity.

The "Bluffing" Story: It exposes makers who are "fake-bidding" to create an
illusion of support, warning the engine that a wall is not "real" and will
crumble upon contact.

The "Vacuum" Story: It identifies a "liquidity vacuum" where one side pulls
away so aggressively that the resulting void "sucks" the price in that
direction.

1. Liquidity Vacuum

One side is retreating and creating a void.
Indicators: High cancel/fill asymmetry with one side retracting.
Semantic Meaning: Vacuum surcharge — the void itself drives price.

2. Toxic Bluff

Near-touch blocks disappear rather than fill.
Indicators: High cancel/fill ratio at near-touch levels.
Semantic Meaning: Manipulated/fake — a bluff wall about to crumble.

3. Hard Support

Liquidity fills rather than cancels on approach.
Indicators: Low cancel/fill ratio (high fill rate) with no side retracting.
Semantic Meaning: Robust/sincere — the wall will hold on contact.

# Summary of Toxicity Categories

| Category         | Cancel/Fill Ratio | Side Retracting | Market "Feel"          |
|:-----------------|:------------------|:----------------|:-----------------------|
| Liquidity Vacuum | High Asymmetry    | One Side        | Vacuum Surcharge       |
| Toxic Bluff      | High              | Near-Touch      | Manipulated / Fake     |
| Hard Support     | Low (High Fill)   | None            | Robust / Sincere       |

---

3. Data source and honest degradation

Cancel-vs-fill is the core metric and is ONLY derivable from L3 per-order events:
the signal seeks `level3/{symbol}/…` in the cadence window (enrichment.go,
`level3Flow`) and labels each `delete` as a cancel (no coincident trade at that
price) or a fill (the tape printed at that level). Bid/ask cancel asymmetry then
infers which side is retreating. Order-level deletes correlated with the trade
tape are what separate a pulled wall from a hit wall — L2 aggregate qty deltas
cannot tell them apart.

There is no historical L3 backfill (authenticated forward capture only), so when
the tree carries no level3 events yet the signal DEGRADES HONESTLY: it falls back
to an L2 churn proxy (top-of-book qty decrease) and clamps confidence to the
fraction of the distribution that L2 alone can support. In L2-only mode it does
NOT claim cancel-vs-fill labels — the churn it measures is "top-of-book quantity
left" with the cancel/fill split unknown, and the measurement is marked l3=0 so
downstream consumers see the degraded basis rather than a fabricated bluff call.

History is tree-only: prior measurement artifacts are the sole replay source
(no local per-pair store); a fresh Signal rebuilds baselines from the tree alone.
*/

/*
Signal analyzes book honesty from cancel-to-fill asymmetry and toxic level
detection. See the struct comment block for category semantics and the L3/L2
honesty contract.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	tree         *dmt.Tree
	batchHistory map[string]toxicityHistory
	batchActive  bool
}

type toxicityHistory struct {
	cancelHistory []float64
	windowStamps  []float64
	prevBidQty    float64
	prevAskQty    float64
}

/*
NewSignal constructs the toxicity signal. The pool parameter is part of the shared
signal constructor contract; this signal does its work inline and does not use it.
*/
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
	return []string{"book", "level3"}
}

func (signal *Signal) ResetBatch() {
	if signal == nil {
		return
	}

	signal.batchHistory = make(map[string]toxicityHistory)
	signal.batchActive = true
}

/*
Measure scores one book frame against its pair's recent touch-depth history and
returns a measurement artifact with a distribution over the three honesty categories.
*/
func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil {
			return
		}

		if datura.Peek[string](datapoint, "channel") != "book" {
			return
		}

		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")
			bidQty := datura.Peek[float64](datapoint, "data", rowIndex, "bids", 0, "qty")
			askQty := datura.Peek[float64](datapoint, "data", rowIndex, "asks", 0, "qty")

			if symbol == "" {
				return
			}

			if bidQty < 0 || askQty < 0 {
				continue
			}

			cancelHistory, windowStamps, prevBidQty, prevAskQty := signal.history(symbol)
			currentStamp := float64(datapoint.Timestamp())
			flow := signal.level3Flow(symbol, windowStamps, currentStamp)

			// L3 (preferred): cancel = order deletes WITHOUT a coincident trade at
			// that price. asymmetry = which side pulled. This is the real honesty
			// metric. l3Basis=1 means the bluff/vacuum/support split is fully earned.
			cancelTotal := flow.cancelTotal()
			asymmetry := flow.asymmetry()
			l3Basis := 1.0

			// L2 degradation: no level3 in the tree. Fall back to top-of-book qty
			// decrease as a churn PROXY only — the cancel/fill split is unknown, so
			// asymmetry is not claimed and confidence is clamped to the fraction of
			// the distribution L2 alone can support (l3Basis derived from how much
			// top-of-book quantity remains relative to its prior, not a magic number).
			if !flow.l3 {
				bidCancel := math.Max(0, prevBidQty-bidQty)
				askCancel := math.Max(0, prevAskQty-askQty)
				cancelTotal = bidCancel + askCancel
				asymmetry = 0.0
				priorTouch := prevBidQty + prevAskQty

				// L2 cannot split the OBSERVED churn into cancel vs fill, so the
				// bluff/vacuum interpretation of that churn is uncertain — discount
				// confidence by the fraction of touch quantity that churned (the
				// ambiguous part). A quiet book (no churn) is fully observable from
				// L2 as sincere support, so it keeps basis 1.
				if priorTouch > 0 && cancelTotal > 0 {
					l3Basis = 1 - cancelTotal/(priorTouch+cancelTotal)
				}
			}

			churnScore := statutil.ScaleByMedianOrUnity(cancelTotal, cancelHistory)
			churnEvidence := churnScore

			if cancelTotal > 0 {
				churnEvidence = math.Max(churnScore, cancelTotal/(1+statutil.Median(cancelHistory)))
			}

			retractionScore := 0.0

			if cancelTotal > 0 {
				retractionScore = cancelTotal / (1 + prevBidQty + prevAskQty)
			}

			churnMass := churnEvidence * (1 + churnEvidence)
			cancelScale := statutil.ScaleByMedianOrUnity(cancelTotal, cancelHistory)
			churnScale := statutil.ScaleByMedianOrUnity(churnMass, cancelHistory)
			supportPenalty := churnMass*churnScale + cancelTotal*cancelScale
			asymmetryWeight := 1 + asymmetry*statutil.ScaleByMedianOrUnity(asymmetry, cancelHistory)
			retractionWeight := retractionScore * retractionScore * statutil.ScaleByMedianOrUnity(retractionScore, cancelHistory)

			shares := []dist.Share{
				{Key: "vacuumScore", Category: logic.CategoryLiquidityVacuum, Mass: retractionScore * asymmetry * asymmetry * churnMass * churnMass * asymmetryWeight},
				{Key: "bluffScore", Category: logic.CategoryToxicBluff, Mass: retractionScore * (1 - asymmetry) * (1 - asymmetry) * churnMass * churnMass * churnMass * (1 + (1 - asymmetry))},
				{Key: "supportScore", Category: logic.CategoryHardSupport, Mass: 1 / (1 + supportPenalty + retractionWeight)},
			}

			measurement := datura.Acquire("toxicity", datura.APPJSON)
			measurement.WithRole("measurement")
			measurement.WithScope(symbol)
			errnie.Error(measurement.SetOrigin(string(logic.SourceToxicity)))
			measurement.SetTimestamp(datapoint.Timestamp())

			output, confidence := dist.Fields(shares)

			// l3 records the honesty basis on the artifact so downstream consumers
			// see whether the cancel/fill split is real (1) or an L2 churn proxy
			// (degraded fraction). The peak-mass confidence is scaled by that basis
			// so an L2-only frame cannot present as a high-confidence bluff call.
			if confidence <= 0 {
				confidence = 0
			}

			output["churn"] = churnScore
			output["asymmetry"] = asymmetry
			output["cancelTotal"] = flow.cancelTotal()
			output["fillTotal"] = flow.fillTotal()
			output["l3"] = l3Basis
			output["confidence"] = confidence * l3Basis
			measurement.MergeOutputs(output)
			measurement.MergeFields(map[string]any{
				"bidQty":      bidQty,
				"askQty":      askQty,
				"cancelTotal": cancelTotal,
				"timestamp":   datapoint.Timestamp(),
			})
			signal.remember(symbol, cancelTotal, bidQty, askQty, currentStamp)

			if !yield(measurement) {
				return
			}
		}
	}
}

/*
history rebuilds the cancel baseline and the prior touch quantities for one
symbol from prior measurement artifacts in the tree alone. windowStamps carries
the observed measurement timestamps so the L3 seek window derives from cadence
(statutil.MedianCadence × WindowDepth), never a fixed horizon. The window depth
of the returned cancel baseline likewise derives from those stamps.
*/
func (signal *Signal) history(symbol string) (
	cancelHistory, windowStamps []float64,
	prevBidQty, prevAskQty float64,
) {
	if signal.tree == nil {
		return nil, nil, 0, 0
	}

	if signal.batchActive && signal.batchHistory != nil {
		if cached, ok := signal.batchHistory[symbol]; ok {
			return cached.cancelHistory,
				cached.windowStamps,
				cached.prevBidQty,
				cached.prevAskQty
		}
	}

	prefix := []byte("measurement/" + symbol + "/" + string(logic.SourceToxicity) + "/")

	var (
		stamps      []float64
		latestStamp float64
	)

	for prior := range signal.tree.Seek(prefix) {
		stamp := datura.Peek[float64](prior, "timestamp")
		cancelHistory = append(cancelHistory, datura.Peek[float64](prior, "cancelTotal"))
		stamps = append(stamps, stamp)

		if stamp < latestStamp {
			continue
		}

		latestStamp = stamp
		prevBidQty = datura.Peek[float64](prior, "bidQty")
		prevAskQty = datura.Peek[float64](prior, "askQty")
	}

	keep := statutil.WindowDepth(stamps)
	history := toxicityHistory{
		cancelHistory: statutil.Tail(cancelHistory, keep),
		windowStamps:  statutil.Tail(stamps, keep),
		prevBidQty:    prevBidQty,
		prevAskQty:    prevAskQty,
	}
	signal.storeHistory(symbol, history)

	return history.cancelHistory,
		history.windowStamps,
		history.prevBidQty,
		history.prevAskQty
}

func (signal *Signal) storeHistory(symbol string, history toxicityHistory) {
	if signal == nil || !signal.batchActive {
		return
	}

	if signal.batchHistory == nil {
		signal.batchHistory = make(map[string]toxicityHistory)
	}

	signal.batchHistory[symbol] = history
}

func (signal *Signal) remember(symbol string, cancelTotal, bidQty, askQty, currentStamp float64) {
	if signal == nil || symbol == "" || currentStamp <= 0 {
		return
	}

	cancelHistory, windowStamps, _, _ := signal.history(symbol)
	windowStamps = append(windowStamps, currentStamp)
	cancelHistory = append(cancelHistory, cancelTotal)
	keep := statutil.WindowDepth(windowStamps)

	signal.storeHistory(symbol, toxicityHistory{
		cancelHistory: statutil.Tail(cancelHistory, keep),
		windowStamps:  statutil.Tail(windowStamps, keep),
		prevBidQty:    bidQty,
		prevAskQty:    askQty,
	})
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
