package hawkes

import (
	"context"
	"iter"
	"math"
	"sort"
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
the bivariate Hawkes fit. This is a trade-tape-only read: the signal ingests the
executed trade stream and measures self-excitation from arrival cadence alone. It
does NOT confirm against book imbalance — there is no L3 order-event ingest wired
here, and aggregated L2 top-of-book quantity cannot honestly distinguish order
intensity (add/delete) from the trade excitation already measured. Book
confirmation is therefore out of scope until level3 ingest exists.

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
See the struct comment block for category semantics. History is tree-only: prior
measurement artifacts are the sole replay source (no local per-pair store), so a
fresh Signal rebuilds baselines from the tree alone.
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
arrivals.
*/
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
			side := datura.Peek[string](datapoint, "data", rowIndex, "side")
			price := datura.Peek[float64](datapoint, "data", rowIndex, "price")
			quantity := datura.Peek[float64](datapoint, "data", rowIndex, "qty")

			if symbol == "" {
				return
			}

			if price <= 0 || quantity <= 0 {
				continue
			}

			stamp := float64(datapoint.Timestamp())
			buyStamps, sellStamps, baseIntensity := signal.history(symbol, datapoint.Timestamp())

			if side == "buy" {
				buyStamps = append(buyStamps, stamp)
			}

			if side == "sell" {
				sellStamps = append(sellStamps, stamp)
			}

			buyIntensity := intensityOf(buyStamps)
			sellIntensity := intensityOf(sellStamps)
			intensity := buyIntensity + sellIntensity

			branching := branchingRatio(append(buyStamps, sellStamps...))

			radiusCap := criticalRadiusCap(append(buyStamps, sellStamps...), branching)
			radius := math.Min(math.Max(0, branching), radiusCap)

			exo := statutil.ScaleByMedianOrUnity(intensity, baseIntensity)
			asymmetry := 0.0

			if intensity > 0 {
				asymmetry = (buyIntensity - sellIntensity) / intensity
			}

			distribution := signal.classify(radius, asymmetry, exo)

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

			dist.Write(measurement, distribution)

			measurement.Merge("side", side)
			measurement.Merge("price", price)
			measurement.Merge("qty", quantity)
			measurement.Merge("intensity", intensity)
			measurement.Merge("timestamp", datapoint.Timestamp())

			if !yield(measurement) {
				return
			}
		}
	}
}

/*
history rebuilds the pair's buy and sell arrival timestamp series plus the recent
background-intensity baseline from prior measurement artifacts in the tree alone,
all scoped to this symbol. The baseline depth is not fixed: it is the most recent
priors spanning a wall-clock window derived from the pair's own arrival cadence
(statutil.WindowDepth), so a pair trading once a minute and one trading ten times
a second are normalised to comparable history. The first observation has no prior
and uses itself as baseline downstream.
*/
func (signal *Signal) history(symbol string, currentStamp int64) (
	buyStamps, sellStamps, baseIntensity []float64,
) {
	if signal.tree == nil {
		return nil, nil, nil
	}

	type priorTrade struct {
		stamp     float64
		side      string
		intensity float64
	}

	samples := make([]priorTrade, 0)
	windowStart := currentStamp - int64(12*time.Hour)
	rolePrefix := "measurement/" + symbol + "/" + string(logic.SourceHawkes)

	for _, seekKey := range dailyPrefixes(rolePrefix, "", windowStart, currentStamp) {
		for prior := range signal.tree.Seek(seekKey) {
			stamp := datura.Peek[float64](prior, "timestamp")
			if stamp == 0 {
				stamp = float64(prior.Timestamp())
			}

			if int64(stamp) < windowStart {
				continue
			}

			if int64(stamp) > currentStamp {
				break
			}

			samples = append(samples, priorTrade{
				stamp:     stamp,
				side:      datura.Peek[string](prior, "side"),
				intensity: datura.Peek[float64](prior, "intensity"),
			})
		}
	}

	// intensityOf and WindowDepth both read the series as time-ordered (first and
	// last stamp form the arrival span), so the Seek order is sorted before use —
	// tree iteration is not guaranteed chronological.
	sort.Slice(samples, func(left, right int) bool {
		return samples[left].stamp < samples[right].stamp
	})

	var (
		buys, sells, intensities, stamps []float64
	)

	for _, sample := range samples {
		if sample.side == "buy" {
			buys = append(buys, sample.stamp)
		}

		if sample.side == "sell" {
			sells = append(sells, sample.stamp)
		}

		intensities = append(intensities, sample.intensity)
		stamps = append(stamps, sample.stamp)
	}

	keep := statutil.WindowDepth(stamps)
	buyStamps = statutil.Tail(buys, keep)
	sellStamps = statutil.Tail(sells, keep)
	baseIntensity = statutil.Tail(intensities, keep)

	return buyStamps, sellStamps, baseIntensity
}

func dailyPrefixes(role string, symbol string, startNano, endNano int64) [][]byte {
	start := time.Unix(0, startNano).UTC().Truncate(24 * time.Hour)
	end := time.Unix(0, endNano).UTC().Truncate(24 * time.Hour)

	if end.Before(start) {
		end = start
	}

	prefixes := make([][]byte, 0, int(end.Sub(start)/(24*time.Hour))+1)

	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		dayStr := cursor.Format("2006/01/02")
		if symbol == "" {
			prefixes = append(prefixes, []byte(role+"/"+dayStr+"/"))
		} else {
			prefixes = append(prefixes, []byte(role+"/"+symbol+"/"+dayStr+"/"))
			prefixes = append(prefixes, []byte(role+"/"+symbol+"/kraken/"+dayStr+"/"))
		}
	}

	return prefixes
}

/*
classify scores the four excitation categories from the spectral radius, the
buy/sell asymmetry and the exogenous (background-scaled) intensity (see the type
comment) and returns raw evidence masses for dist.Write to normalise.
*/
func (signal *Signal) classify(radius, asymmetry, exo float64) []dist.Share {
	directional := math.Abs(asymmetry)

	return []dist.Share{
		{Key: "frenzy", Category: logic.CategoryFrenzy, Mass: directional * directional * radius * (1 + directional)},
		{Key: "saturation", Category: logic.CategorySaturation, Mass: radius * (1 - directional)},
		{Key: "organic", Category: logic.CategoryOrganic, Mass: exo * (1 - radius) * (1 - directional)},
		{Key: "exhaustion", Category: logic.CategoryExhaustion, Mass: math.Max(0, 1-exo)},
	}
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
