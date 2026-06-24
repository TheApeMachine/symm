package causal

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	marketsection "github.com/theapemachine/symm/market"
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
/*
Signal scores causal regimes from trade flow, macro drift, and liquidity stress.
See the package doc for category semantics.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	tree         *dmt.Tree
	CrossSection *marketsection.CrossSection
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	crossSection ...*marketsection.CrossSection,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	section := (*marketsection.CrossSection)(nil)

	if len(crossSection) > 0 {
		section = crossSection[0]
	}

	if section == nil {
		var err error

		section, err = marketsection.NewCrossSection(marketsection.DefaultCrossSectionConfig())

		if err != nil {
			cancel()

			return nil
		}
	}

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		tree:         tree,
		CrossSection: section,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"trade", "ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil {
		return nil
	}

	channel := datura.Peek[string](datapoint, "channel")

	if channel == "ticker" {
		return signal.measureTicker(datapoint)
	}

	if channel != "trade" {
		return nil
	}

	return signal.measureTrade(datapoint)
}

func (signal *Signal) measureTicker(datapoint *datura.Artifact) *datura.Artifact {
	row, rowErr := marketsection.SymbolFromTicker(datapoint)

	if rowErr != nil {
		return nil
	}

	if errnie.Error(signal.CrossSection.Observe(row)) != nil {
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

	measurement := datura.Acquire("causal", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(row.Name)
	errnie.Error(measurement.SetOrigin(string(logic.SourceCausal)))
	measurement.SetTimestamp(datapoint.Timestamp())
	measurement.Merge("spread", spread)
	measurement.Merge("timestamp", datapoint.Timestamp())

	return measurement
}

func (signal *Signal) measureTrade(datapoint *datura.Artifact) *datura.Artifact {
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
	macro := signal.macroDrift(symbol)
	shock := liquidityStress * macro
	beta := macro * (1 + macro) * (1 + velocityScore + macro) * (1 + flowScore) * (1 + flowScore + macro) * (1 + velocityScore)
	uplift := flowScore * velocityScore / (1 + macro*macro + flowScore*flowScore + velocityScore*velocityScore)
	driver := beta + shock + uplift
	noise := 1 / (1 + (driver+beta)*(driver+beta)*40)

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

	return measurement
}

func (signal *Signal) macroDrift(symbol string) float64 {
	window := signal.CrossSection.MaxReturnWindow()
	snapshot := signal.CrossSection.PeerWindowSnapshot(signal.CrossSection.MinBarsRequired(), time.Time{})

	returns := snapshot.MarketReturns

	if len(returns) == 0 {
		returns = signal.CrossSection.SymbolReturns(symbol, window)
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

	sort.Slice(samples, func(leftIndex, rightIndex int) bool {
		return samples[leftIndex].stamp < samples[rightIndex].stamp
	})

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
		sort.Slice(spreadSamples, func(leftIndex, rightIndex int) bool {
			return spreadSamples[leftIndex].stamp < spreadSamples[rightIndex].stamp
		})

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
