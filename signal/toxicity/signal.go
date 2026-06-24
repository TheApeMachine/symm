package toxicity

import (
	"context"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
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
*/
/*
Signal analyzes book honesty from cancel-to-fill asymmetry and toxic level detection.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
}

/*
NewSignal constructs the toxicity signal. The pool parameter is part of the shared
signal constructor contract; this signal does its work inline and does not use it.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
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
	return []string{"book"}
}

/*
Measure scores one book frame against its pair's recent touch-depth history and
returns a measurement artifact with a distribution over the three honesty categories.
*/
func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil {
		return nil
	}

	if datura.Peek[string](datapoint, "channel") != "book" {
		return nil
	}

	symbol := datura.Peek[string](datapoint, "data", 0, "symbol")
	bidQty := datura.Peek[float64](datapoint, "data", 0, "bids", 0, "qty")
	askQty := datura.Peek[float64](datapoint, "data", 0, "asks", 0, "qty")

	if symbol == "" || bidQty < 0 || askQty < 0 {
		return nil
	}

	cancelHistory, prevBidQty, prevAskQty := signal.history(symbol)

	bidCancel := math.Max(0, prevBidQty-bidQty)
	askCancel := math.Max(0, prevAskQty-askQty)
	cancelTotal := bidCancel + askCancel

	churnScore := statutil.ScaleByMedian(cancelTotal, cancelHistory)

	if churnScore == 0 && cancelTotal > 0 {
		churnScore = cancelTotal / (1 + cancelTotal)
	}
	asymmetry := 0.0

	if cancelTotal > 0 {
		asymmetry = math.Abs(bidCancel-askCancel) / cancelTotal
	}

	churnEvidence := churnScore

	if cancelTotal > 0 {
		churnEvidence = math.Max(churnScore, cancelTotal/(1+statutil.Median(cancelHistory)))
	}

	retractionScore := 0.0

	if cancelTotal > 0 {
		retractionScore = cancelTotal / (1 + prevBidQty + prevAskQty)
	}

	churnMass := churnEvidence * (1 + churnEvidence)
	supportPenalty := churnMass*32 + cancelTotal*cancelTotal

	shares := []dist.Share{
		{Key: "vacuumScore", Category: logic.CategoryLiquidityVacuum, Mass: retractionScore * asymmetry * asymmetry * churnMass * churnMass * (1 + asymmetry*2)},
		{Key: "bluffScore", Category: logic.CategoryToxicBluff, Mass: retractionScore * (1 - asymmetry) * (1 - asymmetry) * churnMass * churnMass * churnMass * (1 + (1 - asymmetry))},
		{Key: "supportScore", Category: logic.CategoryHardSupport, Mass: 1 / (1 + supportPenalty + retractionScore*retractionScore*16)},
	}

	measurement := datura.Acquire("toxicity", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceToxicity)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("churn", churnScore)
	measurement.MergeOutput("asymmetry", asymmetry)
	measurement.Merge("bidQty", bidQty)
	measurement.Merge("askQty", askQty)
	measurement.Merge("cancelTotal", cancelTotal)
	measurement.Merge("timestamp", datapoint.Timestamp())

	if len(cancelHistory) == 0 {
		return measurement
	}

	confidence := dist.Write(measurement, shares)

	if confidence <= 0 {
		measurement.Release()

		return nil
	}

	return measurement
}

func (signal *Signal) history(symbol string) (cancelHistory []float64, prevBidQty, prevAskQty float64) {
	if signal.tree == nil {
		return nil, 0, 0
	}

	query := datura.Acquire("toxicity", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceToxicity)))

	defer query.Release()

	var stamps []float64

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		cancelHistory = append(cancelHistory, datura.Peek[float64](prior, "cancelTotal"))
		stamps = append(stamps, datura.Peek[float64](prior, "timestamp"))
		prevBidQty = datura.Peek[float64](prior, "bidQty")
		prevAskQty = datura.Peek[float64](prior, "askQty")
	}

	return statutil.Tail(cancelHistory, statutil.WindowDepth(stamps)), prevBidQty, prevAskQty
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
