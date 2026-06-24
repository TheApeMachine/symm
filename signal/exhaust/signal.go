package exhaust

import (
	"context"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/dist"
	"github.com/theapemachine/symm/statutil"
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

1. Mechanical Collapse — book thinning dominates.
2. Thermal Exhaustion — trade pressure fade dominates.
3. Fragile Expansion — spread widening dominates.
4. Active Reversal — book imbalance flip dominates.

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
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
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
	return []string{"book", "trade"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil {
		return nil
	}

	channel := datura.Peek[string](datapoint, "channel")

	if channel != "book" && channel != "trade" {
		return nil
	}

	symbol := datura.Peek[string](datapoint, "data", 0, "symbol")

	if symbol == "" {
		return nil
	}

	if channel == "trade" {
		return signal.measureTrade(datapoint, symbol)
	}

	return signal.measureBook(datapoint, symbol)
}

func (signal *Signal) measureBook(datapoint *datura.Artifact, symbol string) *datura.Artifact {
	bid := datura.Peek[float64](datapoint, "data", 0, "bids", 0, "price")
	ask := datura.Peek[float64](datapoint, "data", 0, "asks", 0, "price")
	bidQty := datura.Peek[float64](datapoint, "data", 0, "bids", 0, "qty")
	askQty := datura.Peek[float64](datapoint, "data", 0, "asks", 0, "qty")

	if bid <= 0 || ask < bid {
		return nil
	}

	depths, spreads, imbalances, prevDepth, prevSpread, prevImbalance := signal.bookHistory(symbol)

	depth := bidQty + askQty
	spread := ask - bid
	imbalance := 0.0

	if depth > 0 {
		imbalance = (bidQty - askQty) / depth
	}

	thinning := 0.0

	if prevDepth > 0 && depth < prevDepth {
		thinning = (prevDepth - depth) / prevDepth
	}

	widen := 0.0

	if prevSpread > 0 && spread > prevSpread {
		widen = (spread - prevSpread) / prevSpread
	}

	flip := math.Abs(imbalance - prevImbalance)

	thinScore := statutil.ScaleByMedian(thinning, depths)

	if thinning > 0 && len(depths) == 0 {
		thinScore = thinning
	}

	widenScore := statutil.ScaleByMedian(widen, spreads)

	if widen > 0 && len(spreads) == 0 {
		widenScore = widen
	}

	flipScore := statutil.ScaleByMedian(flip, imbalances)

	if flip > 0 && len(imbalances) == 0 {
		flipScore = flip
	}

	pressureFade := signal.peakPressureFade(symbol)

	thermalMass := pressureFade

	if thermalMass > 0 {
		thermalMass = statutil.ScaleByMedian(pressureFade, signal.fadeSamples(symbol))
	}

	shares := []dist.Share{
		{Key: "mechanical", Category: logic.CategoryMechanicalCollapse, Mass: thinScore},
		{Key: "thermal", Category: logic.CategoryThermalExhaustion, Mass: thermalMass},
		{Key: "fragile", Category: logic.CategoryFragileExpansion, Mass: widenScore},
		{Key: "reversal", Category: logic.CategoryActiveReversal, Mass: flipScore},
	}

	urgency := thinScore + widenScore + flipScore + thermalMass

	if urgency <= 0 {
		measurement := signal.baseMeasurement("exhaust", symbol, datapoint)
		measurement.Merge("depth", depth)
		measurement.Merge("spread", spread)
		measurement.Merge("imbalance", imbalance)
		measurement.Merge("pressureFade", pressureFade)
		measurement.Merge("timestamp", datapoint.Timestamp())

		return measurement
	}

	measurement := signal.baseMeasurement("exhaust", symbol, datapoint)
	measurement.MergeOutput("thinning", thinning)
	measurement.MergeOutput("widen", widen)
	measurement.MergeOutput("flip", flip)
	dist.Write(measurement, shares)

	measurement.Merge("depth", depth)
	measurement.Merge("spread", spread)
	measurement.Merge("imbalance", imbalance)
	measurement.Merge("pressureFade", pressureFade)
	measurement.Merge("timestamp", datapoint.Timestamp())

	return measurement
}

func (signal *Signal) measureTrade(datapoint *datura.Artifact, symbol string) *datura.Artifact {
	side := datura.Peek[string](datapoint, "data", 0, "side")
	quantity := datura.Peek[float64](datapoint, "data", 0, "qty")

	if quantity <= 0 {
		return nil
	}

	pressures, prevPressure := signal.tradeHistory(symbol)
	signed := quantity

	if side == "sell" {
		signed = -quantity
	}

	pressure := prevPressure + signed
	fade := 0.0

	if math.Abs(prevPressure) > 0 && math.Abs(pressure) < math.Abs(prevPressure) {
		fade = (math.Abs(prevPressure) - math.Abs(pressure)) / math.Abs(prevPressure)
	}

	fadeScore := statutil.ScaleByMedian(fade, pressures)

	if fade > 0 && len(pressures) == 0 {
		fadeScore = fade
	}

	if fade > 0 && fadeScore == 0 {
		fadeScore = fade / (1 + fade)
	}

	measurement := signal.baseMeasurement("exhaust", symbol, datapoint)
	measurement.Merge("pressure", pressure)
	measurement.Merge("pressureFade", fade)
	measurement.Merge("timestamp", datapoint.Timestamp())

	if len(pressures) == 0 {
		return measurement
	}

	shares := []dist.Share{
		{Key: "mechanical", Category: logic.CategoryMechanicalCollapse, Mass: 0},
		{Key: "thermal", Category: logic.CategoryThermalExhaustion, Mass: fadeScore},
		{Key: "fragile", Category: logic.CategoryFragileExpansion, Mass: 0},
		{Key: "reversal", Category: logic.CategoryActiveReversal, Mass: 0},
	}

	measurement.MergeOutput("pressure", pressure)
	measurement.MergeOutput("pressureFade", fade)
	dist.Write(measurement, shares)

	measurement.Merge("pressure", pressure)
	measurement.Merge("pressureFade", fade)
	measurement.Merge("timestamp", datapoint.Timestamp())

	return measurement
}

func (signal *Signal) baseMeasurement(origin, symbol string, datapoint *datura.Artifact) *datura.Artifact {
	measurement := datura.Acquire(origin, datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceExhaustion)))
	measurement.SetTimestamp(datapoint.Timestamp())

	return measurement
}

func (signal *Signal) fadeSamples(symbol string) []float64 {
	fadeHistory, _ := signal.tradeHistory(symbol)

	return fadeHistory
}

func (signal *Signal) peakPressureFade(symbol string) float64 {
	if signal.tree == nil {
		return 0
	}

	query := datura.Acquire("exhaust", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceExhaustion)))

	defer query.Release()

	var stamps, fades []float64

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		fade := datura.Peek[float64](prior, "pressureFade")
		fades = append(fades, fade)
		stamps = append(stamps, datura.Peek[float64](prior, "timestamp"))
	}

	fades = statutil.Tail(fades, statutil.WindowDepth(stamps))

	peak := 0.0

	for _, fade := range fades {
		if fade > peak {
			peak = fade
		}
	}

	return peak
}

func (signal *Signal) lastMeasurement(symbol string) *datura.Artifact {
	if signal.tree == nil {
		return nil
	}

	query := datura.Acquire("exhaust", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceExhaustion)))

	defer query.Release()

	var last *datura.Artifact

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		last = prior
	}

	return last
}

func (signal *Signal) bookHistory(symbol string) (
	depthDrops, spreadWidens, imbalanceFlips []float64,
	prevDepth, prevSpread, prevImbalance float64,
) {
	last := signal.lastMeasurement(symbol)

	if last == nil {
		return nil, nil, nil, 0, 0, 0
	}

	prevDepth = datura.Peek[float64](last, "depth")
	prevSpread = datura.Peek[float64](last, "spread")
	prevImbalance = datura.Peek[float64](last, "imbalance")

	query := datura.Acquire("exhaust", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceExhaustion)))

	defer query.Release()

	var (
		stamps                               []float64
		lastDepth, lastSpread, lastImbalance float64
	)

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		depth := datura.Peek[float64](prior, "depth")
		spread := datura.Peek[float64](prior, "spread")
		imbalance := datura.Peek[float64](prior, "imbalance")

		if lastDepth > 0 && depth < lastDepth {
			depthDrops = append(depthDrops, (lastDepth-depth)/lastDepth)
		}

		if lastSpread > 0 && spread > lastSpread {
			spreadWidens = append(spreadWidens, (spread-lastSpread)/lastSpread)
		}

		if lastImbalance != 0 || imbalance != 0 {
			imbalanceFlips = append(imbalanceFlips, math.Abs(imbalance-lastImbalance))
		}

		lastDepth = depth
		lastSpread = spread
		lastImbalance = imbalance
		stamps = append(stamps, datura.Peek[float64](prior, "timestamp"))
	}

	keep := statutil.WindowDepth(stamps)

	return statutil.Tail(depthDrops, keep),
		statutil.Tail(spreadWidens, keep),
		statutil.Tail(imbalanceFlips, keep),
		prevDepth,
		prevSpread,
		prevImbalance
}

func (signal *Signal) tradeHistory(symbol string) (fadeHistory []float64, prevPressure float64) {
	last := signal.lastMeasurement(symbol)

	if last != nil {
		prevPressure = datura.Peek[float64](last, "pressure")
	}

	query := datura.Acquire("exhaust", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceExhaustion)))

	defer query.Release()

	var stamps []float64

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		fadeHistory = append(fadeHistory, datura.Peek[float64](prior, "pressureFade"))
		stamps = append(stamps, datura.Peek[float64](prior, "timestamp"))
	}

	return statutil.Tail(fadeHistory, statutil.WindowDepth(stamps)), prevPressure
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
