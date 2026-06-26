package causal

import (
	"context"
	"iter"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
	"github.com/theapemachine/symm/statutil"
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
*/
type symbolHistory struct {
	stamps     []float64
	flows      []float64
	velocities []float64
	spreads    []float64
	lastPrice  float64
}

/*
Signal scores causal regimes from trade flow, macro drift, and liquidity stress.
See the package doc for category semantics.
*/
type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	tree    *dmt.Tree
	symbols sync.Map
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"trade", "ticker"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil {
			return
		}

		channel := datura.Peek[string](datapoint, "channel")

		if channel == "ticker" {
			for rowIndex := 0; ; rowIndex++ {
				if !signal.measureTicker(datapoint, crossSection, rowIndex) {
					return
				}
			}
		}

		if channel != "trade" {
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

func (signal *Signal) measureTicker(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
	rowIndex int,
) bool {
	if crossSection == nil {
		return false
	}

	row, rowErr := market.SymbolFromTicker(datapoint, rowIndex)

	if rowErr != nil {
		return false
	}

	spread := datura.Peek[float64](datapoint, "data", rowIndex, "ask") -
		datura.Peek[float64](datapoint, "data", rowIndex, "bid")

	if spread <= 0 {
		high := datura.Peek[float64](datapoint, "data", rowIndex, "high")
		low := datura.Peek[float64](datapoint, "data", rowIndex, "low")
		spread = high - low
	}

	if spread <= 0 {
		spread = math.Abs(row.Value) * row.Price
	}

	if spread <= 0 {
		return true
	}

	signal.recordSpread(row.Name, float64(datapoint.Timestamp()), spread)

	return true
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

	flowHistory, velocityHistory, spreadHistory, prevPrice := signal.history(symbol)

	velocity := 0.0

	if prevPrice > 0 {
		velocity = math.Log(price / prevPrice)
	}

	liquidityStress := 0.0

	if len(spreadHistory) > 0 {
		currentSpread := spreadHistory[len(spreadHistory)-1]
		baseline := spreadHistory[:len(spreadHistory)-1]
		liquidityStress = statutil.ScaleByMedian(currentSpread, baseline)
	}

	macro := signal.macroDrift(symbol, crossSection)

	// Endogenous Alpha is the causal counterfactual: do(flow = peak aggression)
	// on this symbol's own structural model. Beta is the systemic drift it shares
	// with the sector. Shock is liquidity stress. Noise is the idiosyncratic
	// residual the abduction could not explain. Every mass is a named real
	// quantity — no polynomial of multipliers.
	uplift, residual, counterfactualOK := signal.counterfactual(
		flowHistory,
		velocityHistory,
	)

	// The counterfactual decomposes the move into a causal part (uplift, what
	// do(flow) explains) and a residual (what abduction could not). alpha and
	// noise are the dimensionless fractions of that split — comparable to each
	// other and to the bounded systemic/liquidity scores, instead of mixing raw
	// return units with drift magnitudes.
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

	measurement := datura.Acquire("causal", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceCausal)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("flow", signedFlow)
	measurement.MergeOutput("velocity", velocity)
	measurement.MergeOutput("macro", macro)
	// Signed counterfactual uplift (return units) for the trader's pragmatic
	// value; alpha/noise above are the dimensionless category masses.
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

	if len(spreadHistory) > 0 {
		currentSpread := spreadHistory[len(spreadHistory)-1]
		measurement.Merge("spread", currentSpread)
		measurement.MergeOutput("spread", currentSpread)
	}

	signal.recordTrade(symbol, float64(datapoint.Timestamp()), signedFlow, velocity, price)

	return measurement
}

func (signal *Signal) ensureSymbol(symbol string) *symbolHistory {
	raw, loaded := signal.symbols.Load(symbol)

	if loaded {
		state, ok := raw.(*symbolHistory)

		if ok {
			return state
		}
	}

	state := &symbolHistory{}
	signal.symbols.Store(symbol, state)

	return state
}

func (signal *Signal) recordSpread(symbol string, stamp, spread float64) {
	if spread <= 0 {
		return
	}

	state := signal.ensureSymbol(symbol)
	state.spreads = append(state.spreads, spread)
	state.stamps = append(state.stamps, stamp)
}

func (signal *Signal) recordTrade(
	symbol string,
	stamp, flow, velocity, price float64,
) {
	state := signal.ensureSymbol(symbol)

	if math.Abs(flow) > 0 || len(state.flows) > 0 {
		state.flows = append(state.flows, math.Abs(flow))
	}

	if math.Abs(velocity) > 0 || len(state.velocities) > 0 {
		state.velocities = append(state.velocities, math.Abs(velocity))
	}

	state.stamps = append(state.stamps, stamp)
	state.lastPrice = price
}

func (signal *Signal) macroDrift(
	symbol string,
	crossSection *market.CrossSection,
) float64 {
	if crossSection == nil {
		return 0
	}

	window := crossSection.MaxReturnWindow()
	snapshot := crossSection.PeerWindowSnapshot(crossSection.MinBarsRequired(), time.Time{})

	returns := snapshot.MarketReturns

	if len(returns) == 0 {
		returns = crossSection.SymbolReturns(symbol, window)
	}

	if len(returns) == 0 {
		return 0
	}

	total := 0.0

	for _, value := range returns {
		total += math.Abs(value)
	}

	return total * math.Sqrt(float64(len(returns)))
}

func (signal *Signal) history(symbol string) (
	flowHistory, velocityHistory, spreadHistory []float64,
	prevPrice float64,
) {
	if raw, ok := signal.symbols.Load(symbol); ok {
		state, stateOK := raw.(*symbolHistory)

		if stateOK && len(state.stamps) > 0 {
			keep := statutil.WindowDepth(state.stamps)

			return statutil.Tail(state.flows, keep),
				statutil.Tail(state.velocities, keep),
				statutil.Tail(state.spreads, keep),
				state.lastPrice
		}
	}

	if signal.tree == nil {
		return nil, nil, nil, 0
	}

	query := datura.Acquire("causal", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceCausal)))

	defer query.Release()

	type priorSample struct {
		stamp    float64
		flow     float64
		velocity float64
		spread   float64
		price    float64
	}

	samples := make([]priorSample, 0)
	spreadSamples := make([]priorSample, 0)

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		sample := priorSample{
			stamp:    datura.Peek[float64](prior, "timestamp"),
			flow:     math.Abs(datura.Peek[float64](prior, "flow")),
			velocity: math.Abs(datura.Peek[float64](prior, "velocity")),
			spread:   datura.Peek[float64](prior, "spread"),
			price:    datura.Peek[float64](prior, "price"),
		}

		if sample.spread <= 0 {
			sample.spread = datura.Peek[float64](prior, "output", "spread")
		}

		if sample.spread > 0 {
			spreadSamples = append(spreadSamples, sample)
		}

		if sample.price <= 0 {
			continue
		}

		samples = append(samples, sample)
	}

	if len(samples) == 0 {
		return nil, nil, nil, 0
	}

	stamps := make([]float64, len(samples))

	for index := range samples {
		stamps[index] = samples[index].stamp
	}

	keep := statutil.WindowDepth(stamps)

	if keep <= 0 {
		return nil, nil, nil, 0
	}

	if keep > len(samples) {
		keep = len(samples)
	}

	samples = samples[len(samples)-keep:]

	flowHistory = make([]float64, len(samples))
	velocityHistory = make([]float64, len(samples))
	prevPrice = samples[len(samples)-1].price

	for index := range samples {
		flowHistory[index] = samples[index].flow
		velocityHistory[index] = samples[index].velocity
	}

	if len(spreadSamples) > 0 {
		spreadStamps := make([]float64, len(spreadSamples))

		for index := range spreadSamples {
			spreadStamps[index] = spreadSamples[index].stamp
		}

		spreadKeep := statutil.WindowDepth(spreadStamps)

		if spreadKeep > len(spreadSamples) {
			spreadKeep = len(spreadSamples)
		}

		if spreadKeep > 0 {
			spreadSamples = spreadSamples[len(spreadSamples)-spreadKeep:]
			spreadHistory = make([]float64, len(spreadSamples))

			for index := range spreadSamples {
				spreadHistory[index] = spreadSamples[index].spread
			}
		}
	}

	return flowHistory, velocityHistory, spreadHistory, prevPrice
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
