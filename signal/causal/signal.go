package causal

import (
	"context"
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
) *datura.Artifact {
	if signal == nil || datapoint == nil {
		return nil
	}

	channel := datura.Peek[string](datapoint, "channel")

	if channel == "ticker" {
		return signal.measureTicker(datapoint, crossSection)
	}

	if channel != "trade" {
		return nil
	}

	return signal.measureTrade(datapoint, crossSection)
}

func (signal *Signal) measureTicker(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	if crossSection == nil {
		return nil
	}

	row, rowErr := market.SymbolFromTicker(datapoint)

	if rowErr != nil {
		return nil
	}

	if errnie.Error(crossSection.Observe(row)) != nil {
		return nil
	}

	spread := datura.Peek[float64](datapoint, "data", 0, "ask") -
		datura.Peek[float64](datapoint, "data", 0, "bid")

	if spread <= 0 {
		high := datura.Peek[float64](datapoint, "data", 0, "high")
		low := datura.Peek[float64](datapoint, "data", 0, "low")
		spread = high - low
	}

	if spread <= 0 {
		spread = math.Abs(row.Value) * row.Price
	}

	if spread <= 0 {
		return nil
	}

	signal.recordSpread(row.Name, float64(datapoint.Timestamp()), spread)

	return nil
}

func (signal *Signal) measureTrade(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	symbol := datura.Peek[string](datapoint, "data", 0, "symbol")
	side := datura.Peek[string](datapoint, "data", 0, "side")
	price := datura.Peek[float64](datapoint, "data", 0, "price")
	quantity := datura.Peek[float64](datapoint, "data", 0, "qty")

	if symbol == "" || price <= 0 || quantity <= 0 {
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

	flowScore := statutil.ScaleByMedian(math.Abs(signedFlow), flowHistory)
	velocityScore := statutil.ScaleByMedian(math.Abs(velocity), velocityHistory)
	macro := signal.macroDrift(symbol, crossSection)
	shock := liquidityStress * macro
	beta := macro * (1 + macro) * (1 + velocityScore + macro) * (1 + flowScore) * (1 + flowScore + macro) * (1 + velocityScore)
	uplift := flowScore * velocityScore / (1 + macro*macro + flowScore*flowScore + velocityScore*velocityScore)
	driver := beta + shock + uplift
	noiseScale := macro*macro + flowScore*flowScore + velocityScore*velocityScore + shock*shock
	noise := 1 / (1 + (driver+beta)*(driver+beta)*(1+noiseScale))

	shares := []dist.Share{
		{Key: "alpha", Category: logic.CategoryEndogenousAlpha, Mass: uplift},
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
