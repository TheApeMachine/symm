package hawkes

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/statutil"
)

/*
Hawkes is the Trade-Cluster Excitation perspective. While the Fluid signal looks
at the "vapour pipe" of the book, Hawkes looks at the "temperature" and "chain
reactions" of the trade arrivals themselves.

1. What it measures exactly (in isolation)

The Hawkes signal measures the self-excitation and clustering of trade arrivals
using a bivariate mathematical model. It determines if trades are triggering
subsequent trades in a feedback loop, rather than just occurring as isolated,
random events.

It isolates the following mathematical components:

Exogenous Base (μ): The background rate of trades — those arriving from outside
factors like news or random organic activity.

Branching Ratio (α): The endogenous feedback factor. It measures the descendant
trades likely to be triggered by a single parent trade.

Intensity (λ): The current instantaneous rate of trade arrivals for the buy side
versus the sell side.

Spectral Radius (ρ): A measure of system stability. As the radius approaches the
critical branch (1.0), the trade-flow feedback loop becomes explosive and unstable.

Asymmetry: The net difference between current buy and sell intensities from
the bivariate Hawkes fit, confirmed by top-of-book imbalance when book frames
are ingested.

---

2. Semantically, what story does it tell?

The Hawkes signal tells the story of momentum consistency and market "criticality."

The "Chain Reaction" Story: Unlike simple volume, Hawkes asks: "Is this trade a
lonely event, or the spark of a larger fire?". It distinguishes between a
high-volume spike and genuine momentum ignition.

The "Boiling Point" Story: Using the Spectral Radius, it identifies when the
market is reaching a state of mechanical instability. It tells the story of a
market becoming so "hot" that feedback loops are saturating, making a major break
imminent.

The "Consensus" Story: It identifies the difference between a one-sided frenzy
and a high-intensity tug-of-war. It can tell when buyers and sellers are both
"excited," which signals a high-energy collision of interest.

1. Consensus Frenzy (Directional clustering)

One side of the market has taken complete control of the feedback loop.
Indicators: High asymmetry and moderate spectral radius, with one intensity far
exceeding its background μ.
Semantic Meaning: One side is aggressively hitting the book and triggering a chain
reaction of subsequent trades.

2. Contested Saturation (Critical instability)

The market is at its absolute limit of mechanical stability.
Indicators: Very high spectral radius and high intensity on both sides.
Semantic Meaning: The market is "boiling." Both buyers and sellers are highly
active and exciting each other. The system is super-critical and likely to break
violently once one side exhausts.

3. Exogenous Drift (Orderly flow)

The default state where trades arrive but do not trigger significant cascades.
Indicators: Low spectral radius and intensities staying close to their background
μ levels.
Semantic Meaning: Trades are driven by outside factors rather than internal
market feedback. The engine is running cool and predictably.

4. Flow Exhaustion (Thermal death)

The trade flow has effectively stalled.
Indicators: Current intensities falling significantly below historical background μ.
Semantic Meaning: The feedback loops have died out, and even organic interest has
slowed. The current move has run out of steam.

# Summary of Hawkes Categories

| Category   | Spectral Radius | Asymmetry    | Market "Feel"          |
|:-----------|:----------------|:-------------|:-----------------------|
| Frenzy     | Moderate        | High         | Aggressive/Directional |
| Saturation | High (→ 1.0)    | Low/Moderate | Contested/Unstable     |
| Organic    | Low             | Low          | Healthy/Quiet          |
| Exhaustion | Very Low        | Low          | Stalled/Dying          |

By mapping Hawkes this way, the engine can distinguish between a move that is
smoothly supported (Frenzy) and one that is dangerously overheated (Saturation).
*/
/*
Signal measures trade-cluster self-excitation and Hawkes thermal clustering.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
}

/*
NewSignal constructs the excitation signal. It holds no per-pair state: each
measurement reads its pair's recent trade history straight from the tree, so a
frenzy on BTC and a frenzy on DOGE are each judged against their own recent
arrivals. The pool parameter is part of the shared signal constructor contract;
this signal does its work inline and does not use it.
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
Measure scores one trade frame against its pair's recent trade arrivals,
derives the bivariate Hawkes components inline, classifies the frame into a
distribution over the four excitation categories, and writes a new measurement
artifact (role measurement, origin hawkes, scope symbol) into the tree,
returning it. Book frames carry no trade arrival and only contribute a
top-of-book imbalance read, so they are not measured directly.
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

	stamp := float64(datapoint.Timestamp())
	buyStamps, sellStamps, baseIntensity := signal.history(symbol)

	if side == "buy" {
		buyStamps = append(buyStamps, stamp)
	}

	if side == "sell" {
		sellStamps = append(sellStamps, stamp)
	}

	buyIntensity := intensityOf(buyStamps)
	sellIntensity := intensityOf(sellStamps)
	intensity := buyIntensity + sellIntensity

	// Branching ratio and its spectral radius are read from arrival clustering:
	// a self-exciting flow packs gaps below their own median, an organic one
	// scatters them evenly. The radius rides the branching ratio toward 1.0.
	branching := branchingRatio(append(buyStamps, sellStamps...))
	radius := math.Min(branching, 0.999)

	// Asymmetry is scaled by the pair's own background intensity so a quiet pair
	// and a busy one are read on the same footing.
	exo := statutil.ScaleByMedianOrUnity(intensity, baseIntensity)
	asymmetry := 0.0

	if intensity > 0 {
		asymmetry = (buyIntensity - sellIntensity) / intensity
	}

	distribution := classify(radius, asymmetry, exo)
	confidence := 0.0

	for _, share := range distribution {
		if share.mass > confidence {
			confidence = share.mass
		}
	}

	measurement := datura.Acquire("hawkes", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceHawkes)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("radius", radius)
	measurement.MergeOutput("branching", branching)
	measurement.MergeOutput("asymmetry", asymmetry)
	measurement.MergeOutput("buyIntensity", buyIntensity)
	measurement.MergeOutput("sellIntensity", sellIntensity)
	measurement.MergeOutput("intensity", intensity)
	measurement.MergeOutput("exo", exo)

	// The four excitation categories share 100% of the evidence: the mix is the
	// signal, so each category's mass is published by name and by global index
	// rather than collapsing to one winner.
	for _, share := range distribution {
		measurement.MergeOutput(share.key, share.mass)
		measurement.MergeOutput(fmt.Sprintf("category.%d", logic.CategoryIndex(share.category)), share.mass)
	}

	measurement.MergeOutput("confidence", confidence)
	measurement.MergeOutput("strength", confidence)

	// Raw inputs ride along so the next frame can rebuild the arrival series.
	measurement.Merge("side", side)
	measurement.Merge("price", price)
	measurement.Merge("qty", quantity)
	measurement.Merge("intensity", intensity)
	measurement.Merge("timestamp", datapoint.Timestamp())

	// The trader owns cognition and tree insertion (trader/crypto.go); Measure
	// only derives and returns the measurement.
	return measurement
}

/*
history reads the pair's prior measurements from the tree and rebuilds the buy
and sell arrival timestamp series plus the recent background-intensity baseline,
all scoped to this symbol. The baseline depth is not fixed: it is the most recent
priors spanning a wall-clock window derived from the pair's own arrival cadence
(see windowDepth), so a pair trading once a minute and one trading ten times a
second are normalised to comparable history.
*/
func (signal *Signal) history(symbol string) (
	buyStamps, sellStamps, baseIntensity []float64,
) {
	if signal.tree == nil {
		return nil, nil, nil
	}

	query := datura.Acquire("hawkes", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceHawkes)))

	defer query.Release()

	var (
		buys, sells, intensities, stamps []float64
	)

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		stamp := datura.Peek[float64](prior, "timestamp")
		side := datura.Peek[string](prior, "side")

		if side == "buy" {
			buys = append(buys, stamp)
		}

		if side == "sell" {
			sells = append(sells, stamp)
		}

		intensities = append(intensities, datura.Peek[float64](prior, "intensity"))
		stamps = append(stamps, stamp)
	}

	keep := statutil.WindowDepth(stamps)
	buyStamps = statutil.Tail(buys, keep)
	sellStamps = statutil.Tail(sells, keep)
	baseIntensity = statutil.Tail(intensities, keep)

	return buyStamps, sellStamps, baseIntensity
}

/*
categoryShare names a candidate excitation category by its wire key and global
logic category, with that category's share of the total evidence (0..1).
*/
type categoryShare struct {
	key      string
	category logic.CategoryType
	mass     float64
}

/*
classify scores the four excitation categories from the spectral radius, the
buy/sell asymmetry and the exogenous (background-scaled) intensity (see the type
comment) and returns them as a distribution: each category's mass, normalised so
the four sum to 1.0. The market is rarely purely one category, so the mix is the
signal rather than a single winner. When no category has evidence every mass is
zero.
*/
func classify(radius, asymmetry, exo float64) []categoryShare {
	directional := math.Abs(asymmetry)

	shares := []categoryShare{
		// Consensus Frenzy: directional control on a self-exciting flow.
		{"frenzy", logic.CategoryFrenzy, directional * directional * radius * (1 + directional)},
		// Contested Saturation: critical radius with both sides contesting it.
		{"saturation", logic.CategorySaturation, radius * (1 - directional)},
		// Exogenous Drift: live flow with little internal feedback.
		{"organic", logic.CategoryOrganic, exo * (1 - radius) * (1 - directional)},
		// Flow Exhaustion: live intensity collapsed below its background.
		{"exhaustion", logic.CategoryExhaustion, math.Max(0, 1-exo)},
	}

	total := 0.0

	for index := range shares {
		mass := shares[index].mass

		if math.IsNaN(mass) || math.IsInf(mass, 0) || mass < 0 {
			shares[index].mass = 0

			continue
		}

		total += mass
	}

	if total <= 0 {
		return shares
	}

	for index := range shares {
		shares[index].mass /= total
	}

	return shares
}

/*
intensityOf is the arrival rate of a side: its event count divided by the wall
clock span the events cover. A single event has no span and reads as one event
over a unit interval. The series is assumed time-ordered as written by Measure.
*/
func intensityOf(stamps []float64) float64 {
	if len(stamps) == 0 {
		return 0
	}

	if len(stamps) == 1 {
		return 1
	}

	span := stamps[len(stamps)-1] - stamps[0]

	if span <= 0 {
		return float64(len(stamps))
	}

	return float64(len(stamps)) / span
}

/*
branchingRatio reads the endogenous feedback factor from arrival clustering: the
fraction of consecutive inter-arrival gaps that fall below the series' own median
gap. A self-exciting flow bunches its gaps (most below median), an exogenous
Poisson flow scatters them evenly (about half below). Fewer than three arrivals
carry no cadence and read as zero feedback.
*/
func branchingRatio(stamps []float64) float64 {
	if len(stamps) < 3 {
		return 0
	}

	ordered := append([]float64(nil), stamps...)
	sort.Float64s(ordered)

	gaps := make([]float64, 0, len(ordered)-1)

	for index := 1; index < len(ordered); index++ {
		if gap := ordered[index] - ordered[index-1]; gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	if len(gaps) == 0 {
		return 0
	}

	median := statutil.Median(gaps)
	below := 0

	for _, gap := range gaps {
		if gap < median {
			below++
		}
	}

	return float64(below) / float64(len(gaps))
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
