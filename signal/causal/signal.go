package causal

import (
	"context"
	"io"
	"math"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/qpool"
)

const (
	nodeMacro        = 0
	nodeLiquidity    = 1
	nodeFlow         = 2
	nodeTarget       = 3
	causalMinHistory = 12
)

/*
Causal is the engine’s "rationalist," moving beyond simple
correlations to identify the true structural drivers of
price using Judea Pearl’s "ladder of causation".

1. What it measures exactly (in isolation)

The Causal signal measures the structural relationship between
Macro Momentum, Liquidity, Local Flow, and Price Velocity. It
uses a Directed Acyclic Graph (DAG) to determine if a price move
is an independent event or just a symptom of broader market drift.

It isolates the following causal rungs and metrics:

Rung 1: Association: Measures simple observational correlation
($P(velocity | flow)$).

Rung 2: Intervention: Uses backdoor adjustment to calculate the
effect of "doing" a trade ($P(velocity | do(flow))$) while controlling
for macro and liquidity.

Rung 3: Counterfactual Uplift: Determines what the price move would
have been if the order flow were different than observed.

Structural Regimes: It dynamically switches roles based on market
health. In Normal conditions, "Flow" is the driver; in Panic conditions
(detected via cross-asset Contagion or collinearity), "Liquidity"
itself becomes the driver.

---

2. Semantically, what story does it tell?

The Causal signal tells the story of responsibility and origin.

The "Local vs. Global" Story: It asks: "Is this asset moving because
it's special right now, or because everything is moving?". It filters
out "Macro Drift" to find genuine local alpha.

The "Weaponized Liquidity" Story: It identifies a specific type of
market terror where makers pull quotes so aggressively that the sudden
void itself drives price, while trades merely lag into it.

The "Fragile Equilibrium" Story: By monitoring the Condition Number of
the correlation matrix, it tells the story of a market where flow and
liquidity have collapsed onto a single axis, meaning the structural edges
are no longer identifiable and a regime break is imminent.

1. Endogenous Alpha (The Leader)

The price is being driven by local, internal buying or selling pressure.
Indicators: High Counterfactual Uplift within the Normal (Flow) regime.
Semantic Meaning: The move is "authentic." The local order flow is the
primary cause of price velocity, independent of the rest of the market.

2. Systemic Beta (The Drifter)

The price is moving, but it has no internal driver; it is simply following the tide.
Indicators: High Association (Rung 1) but near-zero Intervention Effect (Rung 2).
Semantic Meaning: The asset is just a passenger. The "cause" is Macro Momentum,
and there is no unique structural reason to favor this specific symbol over the index.

3. Liquidity Shock (The Panic)

The internal mechanics have inverted; the absence of liquidity
is now the dominant force.
Indicators: Panic Regime roles active, triggered by a Contagion
spike toward 1.0 or an exploding Condition Number.
Semantic Meaning: The market is "hollow." Makers have pulled back, and the resulting void is sucking price in. This is a high-risk state where trade flow is a lagging indicator.

4. Causal Noise (The Equilibrium)

No single force—local or macro—has a clear grip on price movement.
Indicators: Low confidence across all causal rungs and high residuals
in the Non-Linear Model.
Semantic Meaning: The market is in a state of stochastic equilibrium.
Neither buyers, sellers, nor the broader macro environment are providing
a statistically significant "push."

# Summary of Causal Categories

| Category         | Active Regime | Dominant Factor       | Market "Feel"      |
|:-----------------|:--------------|:----------------------|:-------------------|
| Endogenous Alpha | Normal        | Counterfactual Uplift | Driven/Independent |
| Systemic Beta    | Normal        | Macro Momentum        | Drifting/Passive   |
| Liquidity Shock  | Panic         | Liquidity Void        | Fragile/Inverted   |
| Causal Noise     | Variable      | None                  | Stochastic/Unclear |

By combining this with the Fluid (mechanical health) and Hawkes (thermal excitation)
signals, the engine can distinguish between a move that is **excited and healthy
(Hawkes Frenzy + Fluid Laminar)** but **causally empty (Systemic Beta)**, versus
a move that is **structurally significant (Endogenous Alpha).**
*/
/*
Signal implements Judea Pearl's ladder of causation over live microstructure feeds.
See the package doc for category semantics.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriteCloser
	pearl       io.ReadWriteCloser
	tree        *dmt.Tree
	schema      *datura.Artifact
}

/*
NewSignal composes the Pearl algorithm preset and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	schema := datura.Acquire("causal", datura.APPJSON).WithAttributes(datura.Map[any]{
		"config": datura.Map[any]{
			"target":            float64(nodeTarget),
			"treatmentNormal":   float64(nodeFlow),
			"controlsNormal":    []float64{float64(nodeMacro), float64(nodeLiquidity)},
			"treatmentInverted": float64(nodeLiquidity),
			"controlsInverted":  []float64{float64(nodeMacro)},
			"conditionLeft":     float64(nodeLiquidity),
			"conditionRight":    float64(nodeFlow),
			"minHistory":        float64(causalMinHistory),
			"contagionSkip":     []float64{float64(nodeMacro), float64(nodeTarget)},
		},
	})

	pearl := algorithm.NewPearl(schema)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		pearl:       pearl,
		algo:        pearl,
		schema:      schema,
		tree:        tree,
	}

	return signal
}

func (signal *Signal) IngestRoles() []string {
	return []string{"trade", "ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil {
		return nil
	}

	channel := datura.Peek[string](datapoint, "channel")

	if channel != "trade" && channel != "ticker" {
		return nil
	}

	if errnie.Error(transport.NewFlipFlop(
		datapoint, signal.algo,
	)) != nil {
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
