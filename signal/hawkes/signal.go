package hawkes

import (
	"context"
	"io"
	"math"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
)

/*
Hawkes is the Trade-Cluster Excitation perspective. While the Fluid signal looks
at the "vapour pipe" of the book, Hawkes looks at the "temperature" and "chain
reactions" of the trade arrivals themselves.

1. What it measures exactly (in isolation)

The Hawkes signal measures the self-excitation and clustering of trade arrivals
using a bivariate mathematical model. It determines if trades are triggering
subsequent trades in a feedback loop, rather than just occurring as isolated,
random events.

It isolates the following mathematical components:

Exogenous Base (μ): The background rate of trades — those arriving from outside
factors like news or random organic activity.

Branching Ratio (α): The endogenous feedback factor. It measures the descendant
trades likely to be triggered by a single parent trade.

Intensity (λ): The current instantaneous rate of trade arrivals for the buy side
versus the sell side.

Spectral Radius (ρ): A measure of system stability. As the radius approaches the
critical branch (1.0), the trade-flow feedback loop becomes explosive and unstable.

Asymmetry: The net difference between current buy and sell intensities from
the bivariate Hawkes fit, confirmed by top-of-book imbalance when book frames
are ingested.

---

2. Semantically, what story does it tell?

The Hawkes signal tells the story of momentum consistency and market "criticality."

The "Chain Reaction" Story: Unlike simple volume, Hawkes asks: "Is this trade a
lonely event, or the spark of a larger fire?". It distinguishes between a
high-volume spike and genuine momentum ignition.

The "Boiling Point" Story: Using the Spectral Radius, it identifies when the
market is reaching a state of mechanical instability. It tells the story of a
market becoming so "hot" that feedback loops are saturating, making a major break
imminent.

The "Consensus" Story: It identifies the difference between a one-sided frenzy
and a high-intensity tug-of-war. It can tell when buyers and sellers are both
"excited," which signals a high-energy collision of interest.

1. Consensus Frenzy (Directional clustering)

One side of the market has taken complete control of the feedback loop.
Indicators: High asymmetry and moderate spectral radius, with one intensity far
exceeding its background μ.
Semantic Meaning: One side is aggressively hitting the book and triggering a chain
reaction of subsequent trades.

2. Contested Saturation (Critical instability)

The market is at its absolute limit of mechanical stability.
Indicators: Very high spectral radius and high intensity on both sides.
Semantic Meaning: The market is "boiling." Both buyers and sellers are highly
active and exciting each other. The system is super-critical and likely to break
violently once one side exhausts.

3. Exogenous Drift (Orderly flow)

The default state where trades arrive but do not trigger significant cascades.
Indicators: Low spectral radius and intensities staying close to their background
μ levels.
Semantic Meaning: Trades are driven by outside factors rather than internal
market feedback. The engine is running cool and predictably.

4. Flow Exhaustion (Thermal death)

The trade flow has effectively stalled.
Indicators: Current intensities falling significantly below historical background μ.
Semantic Meaning: The feedback loops have died out, and even organic interest has
slowed. The current move has run out of steam.

# Summary of Hawkes Categories

| Category   | Spectral Radius | Asymmetry    | Market "Feel"          |
|:-----------|:----------------|:-------------|:-----------------------|
| Frenzy     | Moderate        | High         | Aggressive/Directional |
| Saturation | High (→ 1.0)    | Low/Moderate | Contested/Unstable     |
| Organic    | Low             | Low          | Healthy/Quiet          |
| Exhaustion | Very Low        | Low          | Stalled/Dying          |

By mapping Hawkes this way, the engine can distinguish between a move that is
smoothly supported (Frenzy) and one that is dangerously overheated (Saturation).
*/
/*
Signal measures trade-cluster self-excitation and Hawkes thermal clustering.
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
NewSignal composes the Hawkes excitation pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	excitation := algorithm.NewExcitation(
		datura.Acquire("hawkes-excitation", datura.APPJSON),
	)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			algorithm.NewTradeExcitationSample(
				datura.Acquire("hawkes-trade", datura.APPJSON),
			),
			excitation,
			probability.NewClassifier(
				datura.Acquire("hawkes-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"frenzy", "saturation", "organic", "exhaustion"},
				}),
			),
		),
	}

	return signal
}

func (signal *Signal) IngestRoles() []string {
	return []string{"trade", "book"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil {
		return nil
	}

	channel := datura.Peek[string](datapoint, "channel")

	switch channel {
	case "book":
		if _, err := signal.algo.Write(datapoint.Pack()); err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"hawkes: book write failed",
				err,
			))
		}

		return nil
	case "trade", "":
	default:
		return nil
	}

	if transport.NewFlipFlop(datapoint, signal.algo) != nil {
		return nil
	}

	confidence := datura.Peek[float64](datapoint, "output", "confidence")
	uniformConfidence := 1.0 / 4.0

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
