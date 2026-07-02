package causal

import (
	"context"
	"iter"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
Signal is the engine’s "rationalist," moving beyond simple correlations to identify the true 
structural drivers of price using Judea Pearl’s "ladder of causation".

1. What it measures exactly (in isolation)

The Causal signal measures the structural relationship between Macro Momentum, Liquidity, Local Flow, and Price Velocity. 
It uses a Directed Acyclic Graph (DAG) to determine if a price move is an independent event or just a symptom of broader market drift.

It isolates the following causal rungs and metrics:

* Rung 1: Association: Measures simple observational correlation ($P(velocity | flow)$).
* Rung 2: Intervention: Uses backdoor adjustment to calculate the effect of "doing" a trade ($P(velocity | do(flow))$) while controlling for macro and liquidity.
* Rung 3: Counterfactual Uplift: Determines what the price move *would have been* if the order flow were different than observed.

* Structural Regimes: It dynamically switches roles based on market health. In Normal conditions, "Flow" is the driver; 
in Panic conditions (detected via cross-asset Contagion or collinearity), "Liquidity" itself becomes the driver.

---

2. Semantically, what story does it tell?

The Causal signal tells the story of responsibility and origin.

* The "Local vs. Global" Story: It asks: "Is this asset moving because it's special right now, or because everything is moving?". 
It filters out "Macro Drift" to find genuine local alpha.
* The "Weaponized Liquidity" Story: It identifies a specific type of market terror where makers pull quotes so aggressively that 
the sudden void itself drives price, while trades merely lag into it.
* The "Fragile Equilibrium" Story: By monitoring the Condition Number of the correlation matrix, it tells the story of a market where flow 
and liquidity have collapsed onto a single axis, meaning the structural edges are no longer identifiable and a regime break is imminent.

---

3. Probability Visualization Categories

To map this into a "perspective," we can visualize the probability across these four structural states:

1. Endogenous Alpha (The Leader)

The price is being driven by local, internal buying or selling pressure.
* Indicators: High Counterfactual Uplift within the Normal (Flow) regime.
* Semantic Meaning: The move is "authentic." The local order flow is the primary cause of price velocity, 
independent of the rest of the market.

2. Systemic Beta (The Drifter)

The price is moving, but it has no internal driver; it is simply following the tide.

* Indicators: High Association (Rung 1) but near-zero Intervention Effect (Rung 2).
* Semantic Meaning: The asset is just a passenger. The "cause" is Macro Momentum, and there is no unique structural 
reason to favor this specific symbol over the index.

3. Liquidity Shock (The Panic)

The internal mechanics have inverted; the absence of liquidity is now the dominant force.

* Indicators: Panic Regime roles active, triggered by a Contagion spike toward 1.0 or an exploding Condition Number.
* Semantic Meaning: The market is "hollow." Makers have pulled back, and the resulting void is sucking price in. 
This is a high-risk state where trade flow is a lagging indicator.

4. Causal Noise (The Equilibrium)

No single force—local or macro—has a clear grip on price movement.

* Indicators: Low confidence across all causal rungs and high residuals in the Non-Linear Model.
* Semantic Meaning: The market is in a state of stochastic equilibrium. Neither buyers, sellers, nor the broader 
macro environment are providing a statistically significant "push."

### Summary of Causal Categories

| Category         | Active Regime | Dominant Factor       | Market "Feel"      |
|------------------|---------------|-----------------------|--------------------|
| Endogenous Alpha | Normal        | Counterfactual Uplift | Driven/Independent |
| Systemic Beta    | Normal        | Macro Momentum        | Drifting/Passive   |
| Liquidity Shock  | Panic         | Liquidity Void        | Fragile/Inverted   |
| Causal Noise     | Variable      | None                  | Stochastic/Unclear |

By combining this with the Fluid (mechanical health) and Hawkes (thermal excitation) signals, the engine can 
distinguish between a move that is excited and healthy (Hawkes Frenzy + Fluid Laminar) but causally empty (Systemic Beta), 
versus a move that is structurally significant (Endogenous Alpha).
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
	ticker *Ticker
	book   *Book
	trade  *Trade
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	algo := algorithm.NewPearl(datura.Acquire(
		"causal", datura.APPJSON,
	).WithAttributes(datura.Map[any]{
		"target":            3.0,
		"minHistory":        5.0,
		"treatmentNormal":   2.0,
		"controlsNormal":    []float64{0, 1},
		"treatmentInverted": 1.0,
		"controlsInverted":  []float64{0},
		"conditionLeft":     1.0,
		"conditionRight":    2.0,
		"contagionSkip":     []float64{0, 2, 3},
		"input":             "rawInverted",
		"window":            1.0,
		"categoryIndexes": []float64{
			float64(logic.CategoryIndex(logic.CategoryEndogenousAlpha)),
			float64(logic.CategoryIndex(logic.CategorySystemicBeta)),
			float64(logic.CategoryIndex(logic.CategoryLiquidityShock)),
			float64(logic.CategoryIndex(logic.CategoryCausalNoise)),
		},
	}))

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		ticker: NewTicker(algo),
		book:   NewBook(algo),
		trade:  NewTrade(algo),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker", "book", "trade"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		role := datura.Peek[string](datapoint, "role")

		if role != "ticker" && role != "book" && role != "trade" {
			return
		}

		data := datura.Peek[[]any](datapoint, "data")

		for _, item := range data {
			row, ok := item.(map[string]any)

			if !ok {
				if !yield(datapoint.WithError(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"causal: row object required",
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
					"causal: row symbol required",
					nil,
				)))) {
					return
				}

				continue
			}

			rowArtifact := datura.Acquire(
				"causal", datura.APPJSON,
			).WithRole(
				"measurement",
			).WithScope(
				symbol,
			).WithPayload(
				datura.Map[any](row).Marshal(),
			)
			rowArtifact.SetTimestamp(datapoint.Timestamp())
			errnie.Error(rowArtifact.SetOrigin(string(logic.SourceCausal)))

			switch role {
			case "ticker":
				if !yield(signal.ticker.Measure(rowArtifact, crossSection)) {
					return
				}
			case "book":
				if !yield(signal.book.Measure(rowArtifact, crossSection)) {
					return
				}
			case "trade":
				if !yield(signal.trade.Measure(rowArtifact, crossSection)) {
					return
				}
			}
		}
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
