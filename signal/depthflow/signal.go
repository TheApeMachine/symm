package depthflow

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
DepthFlow is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Multi-level distance weighting is
not wired yet; book math uses top-of-book bid/ask quantities only.

1. Loaded Imbalance — book weight agrees with trade pressure.
2. Spoof Trap — deep-book shape contradicts trade pressure.
3. Book Thinning — defensive depth disappearing at the touch.
4. Dense Neutrality — balanced thick depth with low pressure.

# Summary of DepthFlow Categories

| Category         | WBI (Weighted Imbalance) | Trade Pressure    | Market "Feel"            |
|:-----------------|:-------------------------|:------------------|:-------------------------|
| Loaded Imbalance | High                     | High (Agrees)     | Structural Gravity       |
| Spoof Trap       | High                     | Low (Contradicts) | Manipulated/Fake         |
| Book Thinning    | Rapidly Falling          | Variable          | Exhaustion/Crumbling     |
| Dense Neutrality | Balanced                 | Low               | Robust Stability         |
*/
/*
Signal measures touch-level book imbalance with trade-pressure confirmation.
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

	if channel == "trade" {
		return signal.measureTrade(datapoint)
	}

	if channel != "book" {
		return nil
	}

	symbol := datura.Peek[string](datapoint, "data", 0, "symbol")
	bidQty := datura.Peek[float64](datapoint, "data", 0, "bids", 0, "qty")
	askQty := datura.Peek[float64](datapoint, "data", 0, "asks", 0, "qty")

	if symbol == "" {
		return nil
	}

	depth := bidQty + askQty
	imbalance := 0.0

	if depth > 0 {
		imbalance = (bidQty - askQty) / depth
	}

	wbiHistory, depthHistory, pressureHistory, prevDepth, pressure := signal.history(symbol)

	if len(wbiHistory) == 0 {
		wbi := math.Abs(imbalance)

		measurement := datura.Acquire("depthflow", datura.APPJSON)
		measurement.WithRole("measurement")
		measurement.WithScope(symbol)
		errnie.Error(measurement.SetOrigin(string(logic.SourceDepthFlow)))
		measurement.SetTimestamp(datapoint.Timestamp())
		measurement.MergeOutput("wbi", wbi)
		measurement.Merge("depth", bidQty+askQty)
		measurement.Merge("imbalance", imbalance)
		measurement.Merge("pressure", pressure)
		measurement.Merge("timestamp", datapoint.Timestamp())

		return measurement
	}

	thinning := 0.0

	if prevDepth > 0 && depth < prevDepth {
		thinning = (prevDepth - depth) / prevDepth
	}

	wbi := math.Abs(imbalance)
	wbiScore := statutil.ScaleByMedian(wbi, wbiHistory)
	thinScore := statutil.ScaleByMedian(thinning, depthHistory)
	pressureScore := statutil.ScaleByMedian(math.Abs(pressure), pressureHistory)

	if pressureScore == 0 && math.Abs(pressure) > 0 {
		if len(pressureHistory) > 0 {
			pressureScore = math.Abs(pressure) / (1 + statutil.Median(pressureHistory))
		}

		if pressureScore == 0 {
			pressureScore = math.Abs(pressure) / (1 + math.Abs(pressure))
		}
	}

	aligned := imbalance * pressure
	loadedMass := 0.0
	spoofMass := 0.0

	if aligned >= 0 {
		loadedMass = wbi * pressureScore * (1 + math.Abs(aligned)) * (1 + math.Abs(aligned)) * (1 + math.Abs(pressure)/(1+math.Abs(pressure)))
		spoofMass = wbi / (1 + pressureScore)
	}

	if aligned < 0 {
		spoofMass = wbi * pressureScore * pressureScore * (1 + wbi + pressureScore)
		loadedMass = wbi / (1 + pressureScore)
	}

	shares := []dist.Share{
		{Key: "loaded", Category: logic.CategoryLoadedImbalance, Mass: loadedMass},
		{Key: "spoof", Category: logic.CategorySpoofTrap, Mass: spoofMass},
		{Key: "thinning", Category: logic.CategoryBookThinning, Mass: thinScore},
		{Key: "neutral", Category: logic.CategoryDenseNeutrality, Mass: (1 / (1 + wbiScore*wbiScore*wbiScore)) * (1 / (1 + pressureScore*pressureScore*pressureScore)) * (1 - math.Min(1, math.Abs(aligned)))},
	}

	measurement := datura.Acquire("depthflow", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceDepthFlow)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("wbi", wbi)
	measurement.MergeOutput("pressure", pressure)
	confidence := dist.Write(measurement, shares)

	if confidence <= 0 {
		measurement.Merge("depth", depth)
		measurement.Merge("imbalance", imbalance)
		measurement.Merge("pressure", pressure)
		measurement.Merge("timestamp", datapoint.Timestamp())

		return measurement
	}

	measurement.Merge("depth", depth)
	measurement.Merge("imbalance", imbalance)
	measurement.Merge("pressure", pressure)
	measurement.Merge("timestamp", datapoint.Timestamp())

	return measurement
}

func (signal *Signal) measureTrade(datapoint *datura.Artifact) *datura.Artifact {
	symbol := datura.Peek[string](datapoint, "data", 0, "symbol")
	side := datura.Peek[string](datapoint, "data", 0, "side")
	quantity := datura.Peek[float64](datapoint, "data", 0, "qty")

	if symbol == "" || quantity <= 0 {
		return nil
	}

	signed := quantity

	if side == "sell" {
		signed = -quantity
	}

	measurement := datura.Acquire("depthflow", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceDepthFlow)))
	measurement.SetTimestamp(datapoint.Timestamp())
	measurement.Merge("pressure", signed)
	measurement.Merge("timestamp", datapoint.Timestamp())

	return measurement
}

func (signal *Signal) history(symbol string) (
	wbiHistory, depthHistory, pressureHistory []float64,
	prevDepth, pressure float64,
) {
	if signal.tree == nil {
		return nil, nil, nil, 0, 0
	}

	query := datura.Acquire("depthflow", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceDepthFlow)))

	defer query.Release()

	var stamps []float64
	lastDepth := 0.0

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		wbi := datura.Peek[float64](prior, "output", "wbi")

		if wbi == 0 {
			wbi = math.Abs(datura.Peek[float64](prior, "imbalance"))
		}

		if wbi > 0 {
			wbiHistory = append(wbiHistory, wbi)
		}

		depth := datura.Peek[float64](prior, "depth")

		if lastDepth > 0 && depth < lastDepth {
			depthHistory = append(depthHistory, (lastDepth-depth)/lastDepth)
		}

		lastDepth = depth
		stamps = append(stamps, datura.Peek[float64](prior, "timestamp"))
		prevDepth = depth
		priorPressure := datura.Peek[float64](prior, "pressure")
		pressure = priorPressure

		if priorPressure != 0 {
			pressureHistory = append(pressureHistory, math.Abs(priorPressure))
		}
	}

	keep := statutil.WindowDepth(stamps)

	return statutil.Tail(wbiHistory, keep),
		statutil.Tail(depthHistory, keep),
		statutil.Tail(pressureHistory, keep),
		prevDepth,
		pressure
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
