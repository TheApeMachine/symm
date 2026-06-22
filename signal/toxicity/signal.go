package toxicity

import (
	"context"
	"io"
	"math"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
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
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	tree        *dmt.Tree
}

/*
NewSignal composes the book-quality pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			algorithm.NewBookQualitySample(
				datura.Acquire("toxicity-book", datura.APPJSON).WithAttributes(datura.Map[any]{
					"vacuumGate": datura.Map[any]{
						"percentile": 0.9,
						"minSamples": 3.0,
					},
					"churnGate": datura.Map[any]{
						"percentile": 0.75,
						"minSamples": 3.0,
					},
					"cancelQtyGate": datura.Map[any]{
						"percentile": 0.5,
						"minSamples": 3.0,
					},
					"levelSizeGate": datura.Map[any]{
						"percentile": 0.75,
						"minSamples": 3.0,
					},
					"fillMatchGate": datura.Map[any]{
						"percentile": 0.5,
						"minSamples": 3.0,
					},
					"vacuumLowPercentile": 0.25,
				}),
			),
			equation.NewBookQuality(datura.Acquire("toxicity-bookquality", datura.APPJSON)),
			probability.NewClassifier(
				datura.Acquire("toxicity-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"bluffScore", "vacuumScore", "supportScore"},
				}),
			),
		),
	}

	return signal
}

func (signal *Signal) IngestRoles() []string {
	return []string{"book"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil {
		return nil
	}

	channel := datura.Peek[string](datapoint, "channel")

	if channel != "" && channel != "book" {
		return nil
	}

	if transport.NewFlipFlop(
		datapoint, signal.algo,
	) != nil {
		return nil
	}

	confidence := datura.Peek[float64](datapoint, "output", "confidence")
	uniformConfidence := 1.0 / 3.0

	if confidence <= 0 ||
		math.IsNaN(confidence) ||
		math.IsInf(confidence, 0) ||
		confidence <= uniformConfidence+1e-12 {
		return nil
	}

	return datapoint
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
