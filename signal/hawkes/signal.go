package hawkes

import (
	"context"
	"iter"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market"
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
the bivariate Hawkes fit. This is a trade-tape-only read: the signal ingests the
executed trade stream and measures self-excitation from arrival cadence alone. It
does NOT confirm against book imbalance — there is no L3 order-event ingest wired
here, and aggregated L2 top-of-book quantity cannot honestly distinguish order
intensity (add/delete) from the trade excitation already measured. Book
confirmation is therefore out of scope until level3 ingest exists.

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

type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
	trade  *Trade
}

/*
NewSignal constructs the excitation signal. The tree is held for the shared
signal constructor contract; the trade role owns its rolling artifact clock.
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
		trade:  NewTrade(),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"trade"}
}

/*
Measure routes trade rows into the Hawkes trade-excitation role object.
*/
func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		role := datura.Peek[string](datapoint, "role")

		if role != "trade" {
			return
		}

		data := datura.Peek[[]any](datapoint, "data")

		for _, item := range data {
			row, ok := item.(map[string]any)

			if !ok {
				if !yield(datapoint.WithError(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"hawkes: row object required",
					nil,
				)))) {
					return
				}

				continue
			}

			symbol, ok := row["symbol"].(string)

			if !ok {
				if !yield(datapoint.WithError(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"hawkes: row symbol required",
					nil,
				)))) {
					return
				}

				continue
			}

			rowArtifact := datura.Acquire(
				"hawkes", datura.APPJSON,
			).WithRole(
				"measurement",
			).WithScope(
				symbol,
			).WithPayload(
				datura.Map[any](row).Marshal(),
			)

			measurement := signal.trade.Measure(rowArtifact, crossSection)
			if measurement == nil {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
