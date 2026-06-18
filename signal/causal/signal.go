package causal

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	nomagiquecausal "github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/qpool"
	. "github.com/theapemachine/symm/signal"
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
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	ladder      *algorithm.Pearl
	tree        *dmt.Tree
	nodeStore   *NodeStore
	contagion   *contagionStage
}

/*
NewSignal composes the whole algorithm as one pipeline and subscribes to the
market channels. Data written into algo flows through every stage on its own.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	panel := statistic.NewPanel()
	median := statistic.NewMedian(nil, panel)
	contagion := newContagionStage()

	ladder := algorithm.NewPearl(
		nodeTarget,
		nomagiquecausal.LadderConfig{
			TreatmentNormal:   nodeFlow,
			ControlsNormal:    []int{nodeMacro, nodeLiquidity},
			TreatmentInverted: nodeLiquidity,
			ControlsInverted:  []int{nodeMacro},
			ConditionLeft:     nodeLiquidity,
			ConditionRight:    nodeFlow,
			MinHistory:        causalMinHistory,
		},
		nil,
		contagion,
		nil,
	)

	nodeStore := NewNodeStore()

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		ladder:      ladder,
		nodeStore:   nodeStore,
		contagion:   contagion,
		tree:        tree,
		algo: nomagique.Number(
			panel,
			median,
			contagion,
			ladder,
			probability.NewClassifier(
				ladder.UpliftReading(),
				ladder.ContagionReading(),
				ladder.AssociationReading(),
				ladder.InterventionReading(),
			),
		),
	}

	return signal
}

func (signal *Signal) Measure(query datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	signal.hydrateNodeStoreFromTree()

	signal.ladder.SetNodes(signal.nodeStore.Nodes(scope))

	nodes := signal.nodeStore.Nodes(scope)

	if nodes == nil || nodes.AlignedLength() < causalMinHistory {
		return nil
	}

	if signal.contagion != nil {
		signal.contagion.value = peakNodeMagnitude(nodes)
	}

	processed := datura.Acquire("causal", datura.APPJSON)

	if processed == nil {
		return nil
	}

	processed.WithScope(scope)

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

	InsertMeasurement(signal.tree, processed)

	return processed
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
