package exhaust

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
)

/*
Exhaust is the "Exit Thesis" perspective, tracking microstructure decay to
advise on the urgency of closing an open position.

1. What it measures exactly (in isolation)

The Exhaust signal tracks microstructure decay to advise on the urgency of
closing an open position. Unlike entry signals that look for momentum
ignition, Exhaust looks for momentum rot.

Book Thinning: Measures the trend of bid/ask depth; if depth is disappearing
as price moves, the move is "hollow".

Pressure Fade: Tracks the decay in trade pressure EMA; it signals when the
aggressive "hitters" have run out of ammunition.

Spread Widening: Monitors the bid/ask spread; widening spreads during a trend
indicate increasing mechanical resistance and risk.

Imbalance Flip: Detects when the "weight" of the book flips from the support
side to the resistance side.

---

2. Semantically, what story does it tell?

The Exhaust signal tells the story of when to leave — momentum rot and thesis
decay.

The "Party is Over" Story: It identifies the exact moment a trend stops being
supported by fresh liquidity and begins to rely on "fumes."

The "Trap" Story: By spotting Book Thinning while price is still rising, it
warns that the bid wall is crumbling and a sharp reversal is imminent.

The "Thesis Fade" Story: It provides a "Thesis-decay" exit — if the reason you
entered (imbalance and pressure) is gone, it closes the trade even if the
stop-loss hasn't been hit yet.

1. Mechanical Collapse

Defensive depth is crumbling at the touch.
Indicators: Book thinning as the primary metric.
Semantic Meaning: Crumbling walls/flash-risk — urgent exit.

2. Thermal Exhaustion

Aggressive trade pressure is fading.
Indicators: Pressure fade as the primary metric.
Semantic Meaning: Dying momentum/topping out — soft exit.

3. Fragile Expansion

Spread is widening against the trend.
Indicators: Spread widen as the primary metric.
Semantic Meaning: Increasing friction/risky hold — monitor closely.

4. Active Reversal

Book imbalance has flipped to the other side.
Indicators: Imbalance flip as the primary metric.
Semantic Meaning: Sentiment flip/counter-attack — urgent exit.

# Summary of Exhaust Categories

| Category            | Primary Metric    | Urgency  | Market "Feel"                        |
|:--------------------|:------------------|:---------|:-------------------------------------|
| Mechanical Collapse | Book Thinning     | High     | Crumbling Walls / Flash-Risk         |
| Thermal Exhaustion  | Pressure Fade     | Moderate | Dying Momentum / Topping Out         |
| Fragile Expansion   | Spread Widen      | Moderate | Increasing Friction / Risky Hold     |
| Active Reversal     | Imbalance Flip    | High     | Sentiment Flip / Counter-Attack      |
*/
/*
Signal classifies microstructure decay modes that advise when to close a position.
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
NewSignal composes the decay pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	decay := equation.NewDecay()

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			algorithm.NewDecaySample(
				datura.Acquire("exhaust-book", datura.APPJSON),
			),
			decay,
			probability.NewClassifier(
				datura.Acquire("exhaust-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"mechanical", "fragile", "thermal", "reversal"},
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

	if errnie.Error(transport.NewFlipFlop(
		datapoint, signal.algo,
	)) != nil {
		return nil
	}

	confidence := datura.Peek[float64](datapoint, "output", "confidence")

	if confidence <= 0 || confidence <= 0.25+1e-12 {
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
