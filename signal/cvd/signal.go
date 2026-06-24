package cvd

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
CVD is the Absorption perspective. While the Fluid and Hawkes signals look at the
mechanics and temperature of the book, CVD focuses on the truth of executed volume
to see if a price move is being supported or secretly resisted.

1. What it measures exactly (in isolation)

The CVD signal measures the net difference between aggressor-buy volume and
aggressor-sell volume over a rolling window. It specifically looks for a divergence
between executed flow and price drift.

Net Fraction: The ratio of net volume (buys minus sells) to gross volume (total
trades). A directional read requires a fraction gate derived from observed trade
pressure.

Price Suppression: It measures if the price is staying within a flat band despite
heavy one-sided buying or selling.

Tick Integrity: Because it reads the executed trade tape rather than the book,
it is immune to spoofing.

---

2. Semantically, what story does it tell?

The "Iceberg" Story: It identifies when a massive participant is hidden in the
book, absorbing every market order without letting the price move. It tells us
that what looks like a range-bound market is actually a site of heavy accumulation
or distribution.

The "Authentic Move" Story: It verifies price trends. If price is rising but CVD
is flat or negative, the move is a trap or low-conviction. If price and CVD move
together, the trend is structurally supported.

1. Hidden Absorption

Heavy one-sided flow without corresponding price drift.
Indicators: High net volume with flat price drift.
Semantic Meaning: Bullish/bearish iceberg — hidden accumulation or distribution.

2. Aggressive Drive

Flow and price move together under high net pressure.
Indicators: High net volume with high price drift.
Semantic Meaning: Strong trend support — the tape confirms the move.

3. Stochastic Balance

Low net pressure with no clear directional bias.
Indicators: Low net volume with variable price drift.
Semantic Meaning: Equilibrium/choppy — no dominant aggressor.

4. Volume Starvation

Trade activity has collapsed relative to the rolling baseline.
Indicators: Very low gross volume with flat price drift.
Semantic Meaning: Dying interest — the move has run out of participation.

# Summary of CVD Categories

| Category           | Net Volume | Price Drift | Market "Feel"           |
|:-------------------|:-----------|:------------|:------------------------|
| Hidden Absorption  | High       | Flat        | Bullish/Bearish Iceberg |
| Aggressive Drive   | High       | High        | Strong Trend Support    |
| Stochastic Balance | Low        | Variable    | Equilibrium/Choppy      |
| Volume Starvation  | Very Low   | Flat        | Dying Interest          |
*/
/*
Signal measures cumulative volume delta flow and classifies trade pressure regimes.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
}

/*
NewSignal constructs the CVD signal. The pool parameter is part of the shared
signal constructor contract; this signal does its work inline and does not use it.
*/
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
	return []string{"trade"}
}

/*
Measure scores one trade frame against its pair's recent tape history and returns
a measurement artifact with a distribution over the four flow categories.
*/
func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil {
		return nil
	}

	if datura.Peek[string](datapoint, "channel") != "trade" {
		return nil
	}

	symbol := datura.Peek[string](datapoint, "data", 0, "symbol")
	side := datura.Peek[string](datapoint, "data", 0, "side")
	price := datura.Peek[float64](datapoint, "data", 0, "price")
	quantity := datura.Peek[float64](datapoint, "data", 0, "qty")

	if symbol == "" || price <= 0 || quantity <= 0 {
		return nil
	}

	signedQty := quantity

	if side == "sell" {
		signedQty = -quantity
	}

	grossVolumes, drifts, signedFlows, prevPrice := signal.history(symbol)

	gross := quantity
	netFraction := 0.0

	if gross > 0 {
		netSum := signedQty

		for _, priorSigned := range signedFlows {
			netSum += priorSigned
		}

		grossSum := gross

		for _, priorGross := range grossVolumes {
			grossSum += priorGross
		}

		if grossSum > 0 {
			netFraction = math.Abs(netSum) / grossSum
		}
	}

	drift := 0.0

	if prevPrice > 0 {
		drift = math.Abs(math.Log(price / prevPrice))
	}

	netPressure := statutil.ScaleByMedian(math.Abs(signedQty), grossVolumes)
	driftScore := statutil.ScaleByMedian(drift, drifts)
	grossScore := statutil.ScaleByMedian(gross, grossVolumes)
	flatDrift := 1 / (1 + driftScore)

	absorptionMass := netPressure * flatDrift * (1 - driftScore) * netFraction
	driveMass := netPressure * driftScore * (1 - flatDrift) * netFraction
	balanceMass := flatDrift * flatDrift * (1 - netFraction) / (1 + netPressure)

	starvationMass := math.Max(0, 1-grossScore) * flatDrift * (2 - grossScore)

	shares := []dist.Share{
		{Key: "absorption", Category: logic.CategoryHiddenAbsorption, Mass: absorptionMass},
		{Key: "drive", Category: logic.CategoryAggressiveDrive, Mass: driveMass},
		{Key: "balance", Category: logic.CategoryStochasticBalance, Mass: balanceMass},
		{Key: "starvation", Category: logic.CategoryVolumeStarvation, Mass: starvationMass},
	}

	measurement := datura.Acquire("cvd", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceCVD)))
	measurement.SetTimestamp(datapoint.Timestamp())
	measurement.Merge("side", side)
	measurement.Merge("price", price)
	measurement.Merge("qty", quantity)
	measurement.Merge("signedQty", signedQty)
	measurement.Merge("gross", gross)
	measurement.Merge("drift", drift)
	measurement.Merge("timestamp", datapoint.Timestamp())

	if len(grossVolumes) == 0 {
		return measurement
	}

	measurement.MergeOutput("netFraction", netFraction)
	measurement.MergeOutput("drift", drift)
	measurement.MergeOutput("gross", gross)
	confidence := dist.Write(measurement, shares)

	if confidence <= 0 {
		measurement.Release()

		return nil
	}

	return measurement
}

func (signal *Signal) history(symbol string) (
	grossVolumes, drifts, signedFlows []float64,
	prevPrice float64,
) {
	if signal.tree == nil {
		return nil, nil, nil, 0
	}

	query := datura.Acquire("cvd", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceCVD)))

	defer query.Release()

	var stamps []float64

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		grossVolumes = append(grossVolumes, datura.Peek[float64](prior, "gross"))
		drifts = append(drifts, datura.Peek[float64](prior, "drift"))
		signedFlows = append(signedFlows, datura.Peek[float64](prior, "signedQty"))
		stamps = append(stamps, datura.Peek[float64](prior, "timestamp"))
		prevPrice = datura.Peek[float64](prior, "price")
	}

	keep := statutil.WindowDepth(stamps)

	return statutil.Tail(grossVolumes, keep),
		statutil.Tail(drifts, keep),
		statutil.Tail(signedFlows, keep),
		prevPrice
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
