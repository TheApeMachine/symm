package pumpdump

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
PumpDump is the Ignition perspective, identifying pre-pump microstructure by
looking for sudden "verticality" in volume and price.

1. What it measures exactly (in isolation)

The PumpDump signal identifies pre-pump microstructure by looking for sudden
"verticality" in volume and price.

Volume Lift (RVOL): Measures positive volume delta spikes against a
median-scaled baseline whose depth is derived from the pair's tick cadence.

Precursor Move: Scores upward price detachment from its recent anchor
(positive-only log return, scaled by its own recent median).

Spread Compression: Scores how much the bid/ask spread has tightened versus
its own median-scaled baseline.

Ignition Classifier: Maps rvol, precursor, compression, and rvol-decline into
four ignition states (not a symmetric pump/dump direction classifier).

---

2. Semantically, what story does it tell?

The PumpDump signal tells the story of explosive ignition and coiled energy.

The "Ignition" Story: It identifies the exact moment a move stops being random
walk and becomes a vertical event driven by abnormal volume "lift".

The "Coiled Spring" Story: By tracking spread compression with moderate volume
lift and low precursor, it identifies when a market is "tightly wound" and
ready to snap.

1. Vertical Ignition

Volume and price are detaching together in a vertical event.
Indicators: High volume lift spike with high price precursor.
Semantic Meaning: Launching/Breakout — the move has ignited.

2. Coiled Compression

Energy is building before the vertical move.
Indicators: Moderate volume lift with low price precursor.
Semantic Meaning: Pre-Pump/Loaded — tightly wound and ready to snap.

3. Organic Trend

Steady momentum without abnormal verticality.
Indicators: Low/steady volume lift with moderate price precursor.
Semantic Meaning: Healthy momentum — supported but not explosive.

4. Faded Exhaustion

The vertical leg has lost its lift.
Indicators: Falling volume lift with flat price precursor.
Semantic Meaning: Leg is dead — the ignition has faded.

# Summary of PumpDump Categories

| Category           | Volume Lift | Price Precursor | Market "Feel"            |
|:-------------------|:------------|:----------------|:-------------------------|
| Vertical Ignition  | High Spike  | High            | Launching / Breakout     |
| Coiled Compression | Moderate    | Low             | Pre-Pump / Loaded        |
| Organic Trend      | Low/Steady  | Moderate        | Healthy Momentum         |
| Faded Exhaustion   | Falling     | Flat            | Leg is Dead              |
*/
/*
Signal identifies pre-pump microstructure from volume lift and price verticality.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
}

/*
NewSignal constructs the verticality signal. It holds no per-pair state: each
measurement reads its pair's recent history straight from the tree, so a pump on
BTC and a pump on DOGE are each judged against their own recent measurements.
The pool parameter is part of the shared signal constructor contract; this
signal does its work inline and does not use it.
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
	return []string{"ticker"}
}

/*
Measure scores one ticker frame against its pair's recent history, classifies it
into one of the four ignition categories, and writes a new measurement artifact
(role measurement, origin pumpdump, scope symbol) into the tree, returning it.
*/
func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil {
		return nil
	}

	if datura.Peek[string](datapoint, "channel") != "ticker" {
		return nil
	}

	symbol := datura.Peek[string](datapoint, "data", 0, "symbol")
	bid := datura.Peek[float64](datapoint, "data", 0, "bid")
	ask := datura.Peek[float64](datapoint, "data", 0, "ask")
	last := datura.Peek[float64](datapoint, "data", 0, "last")
	volume := datura.Peek[float64](datapoint, "data", 0, "volume")

	if symbol == "" || last <= 0 || volume <= 0 || ask < bid {
		return nil
	}

	volumes, returns, spreads, prevVolume, prevLast, prevRVOL := signal.history(symbol)

	volumeDelta := math.Max(0, volume-prevVolume)
	signedVolumeDelta := volume - prevVolume
	logReturn := 0.0

	if prevLast > 0 {
		logReturn = math.Max(0, math.Log(last/prevLast))
	}

	// Baselines exclude the current sample so a spike is scored against the
	// pair's prior distribution, not one it has already pulled toward itself.
	rvol := statutil.ScaleByMedian(volumeDelta, volumes)
	precursor := statutil.ScaleByMedian(logReturn, returns)
	compression := statutil.InvertedCompression(ask-bid, spreads)

	if len(volumes) == 0 && volumeDelta > 0 {
		// ponytail: first observation has no prior window; score at unity.
		rvol = 1
	}

	if len(returns) == 0 && prevLast == 0 {
		precursor = 1
	}

	if len(spreads) == 0 && ask > bid {
		compression = 1
	}
	rvolDecline := 0.0

	if prevRVOL > 0 && rvol < prevRVOL {
		rvolDecline = (prevRVOL - rvol) / prevRVOL
	}

	if len(volumes) > 0 {
		baseline := statutil.Median(volumes)

		if baseline > 0 {
			relativeVolume := volumeDelta / baseline

			if relativeVolume < 1 {
				volumeDecline := 1 - relativeVolume

				if volumeDecline > rvolDecline {
					rvolDecline = volumeDecline
				}
			}
		}
	}

	if signedVolumeDelta < 0 && prevVolume > 0 {
		volumeDrop := math.Abs(signedVolumeDelta) / prevVolume

		if volumeDrop > rvolDecline {
			rvolDecline = volumeDrop
		}
	}

	distribution := classify(rvol, precursor, compression, rvolDecline, signal.ignitionFloor(symbol))

	measurement := datura.Acquire("pumpdump", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourcePumpDump)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("rvol", rvol)
	measurement.MergeOutput("precursor", precursor)
	measurement.MergeOutput("compression", compression)
	measurement.MergeOutput("rvolDecline", rvolDecline)
	measurement.MergeOutput("spread", ask-bid)

	// The four ignition categories share 100% of the evidence: the mix is the
	// signal, so each category's mass is published by name and by global index.
	dist.Write(measurement, distribution)

	// Raw inputs ride along so the next frame can derive deltas/returns.
	measurement.Merge("volume", volume)
	measurement.Merge("last", last)
	measurement.Merge("spread", ask-bid)
	measurement.Merge("volumeDelta", volumeDelta)
	measurement.Merge("logReturn", logReturn)
	measurement.Merge("rvol", rvol)
	measurement.Merge("timestamp", datapoint.Timestamp())

	// The trader owns cognition and tree insertion (trader/crypto.go); Measure
	// only derives and returns the measurement.
	return measurement
}

/*
history reads the pair's prior measurements from the tree and rebuilds the
volume-delta, log-return and spread baselines plus the previous frame's raw
volume, last price and rvol, all scoped to this symbol. The baseline depth is
not fixed: it is the most recent priors spanning a wall-clock window derived
from the pair's own tick cadence (see windowSpan), so a pair ticking once a
minute and one ticking ten times a second are normalised to comparable history.
*/
func (signal *Signal) history(symbol string) (
	volumes, returns, spreads []float64,
	prevVolume, prevLast, prevRVOL float64,
) {
	if signal.tree == nil {
		return nil, nil, nil, 0, 0, 0
	}

	query := datura.Acquire("pumpdump", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourcePumpDump)))

	defer query.Release()

	var (
		deltas, rets, sprs []float64
		stamps             []float64
		latestStamp        float64
	)

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		stamp := datura.Peek[float64](prior, "timestamp")
		deltas = append(deltas, datura.Peek[float64](prior, "volumeDelta"))
		rets = append(rets, datura.Peek[float64](prior, "logReturn"))
		sprs = append(sprs, datura.Peek[float64](prior, "spread"))
		stamps = append(stamps, stamp)

		if stamp < latestStamp {
			continue
		}

		latestStamp = stamp
		prevVolume = datura.Peek[float64](prior, "volume")
		prevLast = datura.Peek[float64](prior, "last")
		prevRVOL = datura.Peek[float64](prior, "output", "rvol")

		if prevRVOL == 0 {
			prevRVOL = datura.Peek[float64](prior, "rvol")
		}
	}

	keep := statutil.WindowDepth(stamps)
	volumes = statutil.Tail(deltas, keep)
	returns = statutil.Tail(rets, keep)
	spreads = statutil.Tail(sprs, keep)

	return volumes, returns, spreads, prevVolume, prevLast, prevRVOL
}

/*
classify scores the four ignition categories from the three signals plus rvol
decline (see the type comment) and returns raw evidence masses for dist.Write.
*/
func classify(
	rvol, precursor, compression, rvolDecline, ignitionFloor float64,
) []dist.Share {
	lift := math.Max(0, rvol-1)
	liftSignal := lift / (1 + ignitionFloor)
	hotPrecursor := precursor / (1 + precursor)
	steadyFlow := rvol / (1 + rvol)
	trendMass := hotPrecursor * (1 + hotPrecursor) / (1 + liftSignal*liftSignal*4 + lift)
	flatMass := steadyFlow / (1 + liftSignal*8 + hotPrecursor*2)

	liftEdge := math.Max(0, liftSignal-hotPrecursor)

	declineEdge := math.Max(0, rvolDecline-hotPrecursor-liftSignal*0.25)
	compressionMass := compression * rvol * (1 + compression) * (1 - hotPrecursor) * (1 - declineEdge) / (1 + hotPrecursor)
	exhaustionMass := declineEdge * (1 + declineEdge) * (1 + declineEdge) / (1 + hotPrecursor + compression*(1-compression))

	return []dist.Share{
		{Key: "ignition", Category: logic.CategoryVerticalIgnition, Mass: liftEdge * hotPrecursor * (1 + liftSignal + lift) * (1 + hotPrecursor)},
		{Key: "compression", Category: logic.CategoryCoiledCompression, Mass: compressionMass},
		{Key: "trend", Category: logic.CategoryOrganicTrend, Mass: trendMass + flatMass},
		{Key: "exhaustion", Category: logic.CategoryFadedExhaustion, Mass: exhaustionMass},
	}
}

func (signal *Signal) ignitionFloor(symbol string) float64 {
	if signal.tree == nil {
		return 0
	}

	query := datura.Acquire("pumpdump", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourcePumpDump)))

	defer query.Release()

	lifts := make([]float64, 0, statutil.SampleBudget)

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		priorRVOL := datura.Peek[float64](prior, "rvol")

		if priorRVOL <= 0 {
			continue
		}

		lifts = append(lifts, math.Max(0, priorRVOL-1))
	}

	return statutil.Median(lifts)
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
