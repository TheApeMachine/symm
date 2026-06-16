package hawkes

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

/*
Signal is the Hawkes signal, which focuses on the Trade-Cluster Excitation
perspective. While the Fluid signal looks at the "vapour pipe" of the book,
Hawkes looks at the "temperature" and "chain reactions" of the trade arrivals
themselves.

What it measures exactly (in isolation)

The Hawkes signal measures the self-excitation and clustering of trade
arrivals using a bivariate mathematical model. It determines if trades are
triggering subsequent trades in a feedback loop, rather than just occurring
as isolated, random events.

It isolates the following mathematical components:

Exogenous Base ($\mu$): The "background" rate of trades—those arriving from
outside factors like news or random organic activity.

Branching Ratio ($\alpha$): The endogenous feedback factor. It measures
the "descendant" trades likely to be triggered by a single "parent" trade.

Intensity ($\lambda$): The current instantaneous rate of trade arrivals
for the buy side versus the sell side.

Spectral Radius ($\rho$): A measure of system stability. As the radius
approaches the **"critical branch" (1.0)**, the trade-flow feedback loop
becomes explosive and unstable.

Asymmetry: The net difference between current buy and sell intensities,
further confirmed by top-of-book imbalance.

---

Semantically, what story does it tell?

The Hawkes signal tells the story of momentum consistency and market "criticality."

The "Chain Reaction" Story: Unlike simple volume, Hawkes asks:
"Is this trade a lonely event, or the spark of a larger fire?".
It distinguishes between a high-volume spike and genuine momentum ignition.

The "Boiling Point" Story: Using the Spectral Radius, it identifies when
the market is reaching a state of mechanical instability. It tells the
story of a market becoming so "hot" that feedback loops are saturating,
making a major break imminent.

The "Consensus" Story: It identifies the difference between a one-sided
frenzy and a high-intensity tug-of-war. It can tell when buyers and sellers
are both "excited," which signals a high-energy collision of interest.

---

# Probability Visualization Categories

To map this signal into a "perspective," we can visualize the probability
across these four mechanical states:

1. Consensus Frenzy (Directional clustering)

One side of the market has taken complete control of the feedback loop.

Indicators: High Asymmetry and moderate Spectral Radius, with one intensity
far exceeding its background $\mu$.

Semantic Meaning: One side is aggressively hitting the book and triggering
a chain reaction of subsequent trades.

2. Contested Saturation (Critical instability)

The market is at its absolute limit of mechanical stability.

Indicators: Very high Spectral Radius ($>0.85$) and high Intensity on *both* sides.

Semantic Meaning: The market is "boiling." Both buyers and sellers are highly
active and exciting each other. The system is "super-critical" and likely to
break violently once one side exhausts.

3. Exogenous Drift (Orderly flow)

The default state where trades arrive but do not trigger significant cascades.

Indicators: Low Spectral Radius and intensities staying close to their
background $\mu$ levels.

Semantic Meaning: Trades are driven by outside factors rather than internal
market feedback. The "engine" is running cool and predictably.

4. Flow Exhaustion (Thermal death)

The trade flow has effectively stalled.

Indicators: Current intensities falling significantly below historical
background $\mu$.

Semantic Meaning: The feedback loops have died out, and even organic interest
has slowed. The current move has "run out of steam."

# Summary of Hawkes Categories

| Category   | Spectral Radius   | Asymmetry    | Market "Feel"          |
|------------|-------------------|--------------|------------------------|
| Frenzy     | Moderate          | High         | Aggressive/Directional |
| Saturation | High ($ \to 1.0$) | Low/Moderate | Contested/Unstable     |
| Organic    | Low               | Low          | Healthy/Quiet          |
| Exhaustion | Very Low          | Low          | Stalled/Dying          |

By mapping Hawkes this way, the engine can distinguish between a move that is
smoothly supported (Frenzy) and one that is dangerously overheated (Saturation).
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	excitation  *algorithm.Excitation
	classifier  *probability.Classifier
	trade       *Trade
}

/*
NewSignal composes the Hawkes pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	excitation := algorithm.NewExcitation()
	classifier := probability.NewClassifier(
		excitation.FrenzyReading(),
		excitation.SaturationReading(),
		excitation.OrganicReading(),
		excitation.ExhaustionReading(),
	)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		excitation:  excitation,
		classifier:  classifier,
		algo: nomagique.Number(
			excitation,
			classifier,
			probability.NewTransitionSurprise(5, 1.0/float64(feedRingCapacity)),
		),
		trade: NewTrade(ctx),
	}

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch artifact.Peek("role") {
	case "trade":
		signal.trade.Update(
			datura.As[krakenmarket.TradeUpdates](artifact),
		)
	case "measurement":
		signal.Measure(artifact)
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := in.Peek("scope")
	signal.trade.scope = scope

	frame := make([]byte, 8192)

	readCount, readErr := signal.trade.Read(frame)

	if readCount == 0 {
		return logic.Measurement{}, nil
	}

	if readErr != nil && readErr != io.EOF {
		return logic.Measurement{}, readErr
	}

	if _, err := signal.algo.Write(frame[:readCount]); err != nil {
		return logic.Measurement{}, err
	}

	out := datura.Acquire("hawkes-out", datura.Artifact_Type_json)

	outCount, err := signal.algo.Read(frame)

	if err != nil && err != io.EOF {
		return logic.Measurement{}, err
	}

	if _, err := out.Write(frame[:outCount]); err != nil {
		return logic.Measurement{}, err
	}

	if !signal.excitation.Outcome().Eligible {
		return logic.Measurement{}, nil
	}

	categoryIndex := signal.classifier.CategoryIndex()

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	confidence, confidenceErr := signal.classifier.Confidence(categoryIndex)

	if confidenceErr != nil {
		return logic.Measurement{}, confidenceErr
	}

	snapshot := signal.trade.Snapshot(scope)

	return logic.Measurement{
		Source:     logic.SourceHawkes,
		Symbol:     scope,
		Price:      snapshot.Price,
		Strength:   signal.excitation.Outcome().Strength,
		Volume:     snapshot.Volume,
		Spread:     0,
		Elapsed:    snapshot.Elapsed,
		Category:   hawkesCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](out, "transition.surprise"),
		ObservedAt: snapshot.Observed,
	}, nil
}

func hawkesCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryFrenzy
	case 2:
		return logic.CategorySaturation
	case 3:
		return logic.CategoryOrganic
	case 4:
		return logic.CategoryExhaustion
	default:
		return logic.CategoryOrganic
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
