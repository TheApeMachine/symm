package cvd

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
CVD is the Absorption perspective. While the Fluid and Hawkes signals look at the
mechanics and temperature of the book, CVD focuses on the truth of executed volume
to see if a price move is being supported or secretly resisted.

1. What it measures exactly (in isolation)

The CVD signal measures the net difference between aggressor-buy volume and
aggressor-sell volume over a rolling window. It specifically looks for a divergence
between executed flow and price drift.

Net Fraction: The ratio of net volume (buys minus sells) to gross volume (total
trades). A directional read requires a fraction gate derived from observed trade
pressure.

Price Suppression: It measures if the price is staying within a flat band despite
heavy one-sided buying or selling.

Tick Integrity: Because it reads the executed trade tape rather than the book,
it is immune to spoofing.

---

2. Semantically, what story does it tell?

The "Iceberg" Story: It identifies when a massive participant is hidden in the
book, absorbing every market order without letting the price move. It tells us
that what looks like a range-bound market is actually a site of heavy accumulation
or distribution.

The "Authentic Move" Story: It verifies price trends. If price is rising but CVD
is flat or negative, the move is a trap or low-conviction. If price and CVD move
together, the trend is structurally supported.

1. Hidden Absorption

Heavy one-sided flow without corresponding price drift.
Indicators: High net volume with flat price drift.
Semantic Meaning: Bullish/bearish iceberg — hidden accumulation or distribution.

2. Aggressive Drive

Flow and price move together under high net pressure.
Indicators: High net volume with high price drift.
Semantic Meaning: Strong trend support — the tape confirms the move.

3. Stochastic Balance

Low net pressure with no clear directional bias.
Indicators: Low net volume with variable price drift.
Semantic Meaning: Equilibrium/choppy — no dominant aggressor.

4. Volume Starvation

Trade activity has collapsed relative to the rolling baseline.
Indicators: Very low gross volume with flat price drift.
Semantic Meaning: Dying interest — the move has run out of participation.

# Summary of CVD Categories

| Category           | Net Volume | Price Drift | Market "Feel"           |
|:-------------------|:-----------|:------------|:------------------------|
| Hidden Absorption  | High       | Flat        | Bullish/Bearish Iceberg |
| Aggressive Drive   | High       | High        | Strong Trend Support    |
| Stochastic Balance | Low        | Variable    | Equilibrium/Choppy      |
| Volume Starvation  | Very Low   | Flat        | Dying Interest          |
*/
/*
Signal measures cumulative volume delta flow and classifies trade pressure regimes.
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
NewSignal composes the CVD flow pipeline for tree replay measurement.
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
			algorithm.NewTradeFlowSample(
				datura.Acquire("cvd-trade", datura.APPJSON),
			),
			equation.NewFlow(datura.Acquire("cvd-flow", datura.APPJSON)),
			probability.NewClassifier(
				datura.Acquire("cvd-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"absorption", "drive", "balance", "starvation"},
				}),
			),
		),
	}

	return signal
}

func (signal *Signal) IngestRoles() []string {
	return []string{"trade"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil {
		return nil
	}

	channel := datura.Peek[string](datapoint, "channel")

	if channel != "" && channel != "trade" {
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
