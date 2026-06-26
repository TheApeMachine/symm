package causal

import (
	"context"
	"iter"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
)

/*
Causal is the engine's rationalist perspective on structural drivers of price.

# Summary of Causal Categories

| Category         | Active Regime | Dominant Factor       | Market "Feel"      |
|:-----------------|:--------------|:----------------------|:-------------------|
| Endogenous Alpha | Normal        | Counterfactual Uplift | Driven/Independent |
| Systemic Beta    | Normal        | Macro Momentum        | Drifting/Passive   |
| Liquidity Shock  | Panic         | Liquidity Void        | Fragile/Inverted   |
| Causal Noise     | Variable      | None                  | Stochastic/Unclear |

Liquidity Shock is derived from the live L2 book (touch spread scaled against
the symbol's own spread baseline, amplified by book void) — not the ticker
summary. Endogenous Alpha is the abductive counterfactual on this symbol's own
trade history; Systemic Beta is the cross-section macro drift; Causal Noise is
the unexplained residual. History lives in the tree alone (historian), so the
next frame rebuilds every window from prior measurements without a local cache.
*/
type Signal struct {
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	tree      *dmt.Tree
	historian historian
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:       ctx,
		cancel:    cancel,
		tree:      tree,
		historian: NewHistorian(tree),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"trade", "book"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil {
			return
		}

		if datura.Peek[string](datapoint, "channel") != "trade" {
			return
		}

		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")

			if symbol == "" {
				return
			}

			measurement := signal.measureTrade(datapoint, crossSection, rowIndex)

			if measurement == nil {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) measureTrade(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
	rowIndex int,
) *datura.Artifact {
	symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")
	side := datura.Peek[string](datapoint, "data", rowIndex, "side")
	price := datura.Peek[float64](datapoint, "data", rowIndex, "price")
	quantity := datura.Peek[float64](datapoint, "data", rowIndex, "qty")

	if price <= 0 || quantity <= 0 {
		return nil
	}

	signedFlow := quantity

	if side == "sell" {
		signedFlow = -quantity
	}

	currentStamp := float64(datapoint.Timestamp())
	flowHistory, velocityHistory, prevPrice := signal.historian.window(symbol)

	velocity := 0.0

	if prevPrice > 0 {
		velocity = math.Log(price / prevPrice)
	}

	liquidityStress, bookSpread := signal.historian.bookStress(symbol, currentStamp)
	macro := signal.historian.macroDrift(symbol, crossSection)

	// Endogenous Alpha is the causal counterfactual: do(flow = peak aggression)
	// on this symbol's own structural model. Beta is the systemic drift it shares
	// with the sector. Shock is book-derived liquidity stress. Noise is the
	// idiosyncratic residual the abduction could not explain. Every mass is a
	// named real quantity — no polynomial of multipliers.
	uplift, residual, counterfactualOK := signal.counterfactual(
		flowHistory,
		velocityHistory,
	)

	// The counterfactual decomposes the move into a causal part (uplift, what
	// do(flow) explains) and a residual (what abduction could not). alpha and
	// noise are the dimensionless fractions of that split — comparable to each
	// other and to the bounded systemic/liquidity scores.
	explained := math.Abs(uplift)
	unexplained := math.Abs(residual)
	total := explained + unexplained

	alpha := 0.0
	noise := 1.0

	if counterfactualOK && total > 0 {
		alpha = explained / total
		noise = unexplained / total
	}

	beta := macro / (1 + macro)
	shock := liquidityStress / (1 + liquidityStress)

	shares := []dist.Share{
		{Key: "alpha", Category: logic.CategoryEndogenousAlpha, Mass: alpha},
		{Key: "beta", Category: logic.CategorySystemicBeta, Mass: beta},
		{Key: "shock", Category: logic.CategoryLiquidityShock, Mass: shock},
		{Key: "noise", Category: logic.CategoryCausalNoise, Mass: noise},
	}

	return signal.emit(
		datapoint,
		symbol,
		shares,
		signedFlow,
		velocity,
		macro,
		uplift,
		noise,
		price,
		liquidityStress,
		bookSpread,
	)
}

/*
emit assembles the measurement artifact with its category distribution, output
scalars, and the replay fields (flow, velocity, macro, uplift, noise, spread,
price, timestamp) the next frame rebuilds state from.
*/
func (signal *Signal) emit(
	datapoint *datura.Artifact,
	symbol string,
	shares []dist.Share,
	signedFlow, velocity, macro, uplift, noise, price, liquidityStress, bookSpread float64,
) *datura.Artifact {
	measurement := datura.Acquire("causal", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceCausal)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("flow", signedFlow)
	measurement.MergeOutput("velocity", velocity)
	measurement.MergeOutput("macro", macro)
	// Signed counterfactual uplift (return units) for the trader's pragmatic
	// value; alpha/noise are the dimensionless category masses.
	measurement.MergeOutput("uplift", uplift)
	measurement.MergeOutput("noise", noise)

	confidence := dist.Write(measurement, shares)

	if confidence <= 0 {
		measurement.Release()

		return nil
	}

	measurement.Merge("price", price)
	measurement.Merge("flow", signedFlow)
	measurement.Merge("velocity", velocity)
	measurement.Merge("timestamp", datapoint.Timestamp())

	// Persist the raw book touch spread as the replay baseline the next frame
	// scales against; expose the derived stress as the shock output scalar.
	if bookSpread > 0 {
		measurement.Merge("spread", bookSpread)
		measurement.MergeOutput("spread", bookSpread)
	}

	if liquidityStress > 0 {
		measurement.MergeOutput("shock", liquidityStress)
	}

	return measurement
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
