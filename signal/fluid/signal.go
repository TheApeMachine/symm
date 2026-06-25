package fluid

import (
	"context"
	"fmt"
	"iter"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/statutil"
	"gonum.org/v1/gonum/stat"
)

/*
Fluid is the mechanical perspective on the order book, mapping
microstructural metrics — Reynolds Number (Re), Divergence (Div),
Vorticity (Vort), and Turbulence (Turb) — against Viscosity (Visc).

1. What it measures exactly (in isolation)

The Fluid signal applies order-book fluid dynamics per symbol from book,
trades, and ticks. Reynolds classifies laminar versus turbulent flow.
Divergence is ∇·(ρv) at the touch. Viscosity is replenishment resistance
after consumption.

It isolates the following mechanical states:

Laminar Stability (Orderly Flow): High Viscosity (tight bid/ask spreads)
coupled with low Field Activity.

Turbulent Chaos (Mechanical Breakdown): Dominant Turbulence readings
(Turb) and high Vorticity (Vort).

Inertial Displacement (Directional Surge): A high Reynolds Number (Re)
and high Divergence (Div).

Viscous Resistance (The "Grind"): Low Viscosity (wide spreads/high
resistance) with moderate Divergence. Price memory (fractional-diff proxy
from recent last-price span) reinforces viscous scoring when replenishment
lags displacement.

---

2. Semantically, what story does it tell?

The Fluid signal tells the story of mechanical health — whether the
"vapour pipe" of the market is running smoothly or shattering.

The "Smooth Pipe" Story: Price moves are smooth and the book absorbs
updates without churning. The market is at a constant, manageable diameter.

The "Shattered Mechanics" Story: High turbulence and vorticity readings
signal genuine microstructural chaos rather than price volatility alone.

The "Grind" Story: Every tick move requires a massive amount of "work"
(traded volume), but spread resistance keeps displacement contained.

1. Laminar Stability (Orderly Flow)

The "vapour pipe" of the market is at a constant, manageable diameter.
Indicators: High Viscosity (tight spreads) coupled with low Field Activity.
Semantic Meaning: Price moves are smooth, and the book is absorbing updates
without churning.

2. Turbulent Chaos (Mechanical Breakdown)

The internal mechanics of the market are "shattering," often preceding a
major regime shift.
Indicators: Dominant Turbulence readings and high Vorticity.
Semantic Meaning: Genuine microstructural chaos rather than just price
volatility.

3. Inertial Displacement (Directional Surge)

The market is being forcibly "pushed" by one-sided order flow.
Indicators: A high Reynolds Number and high Divergence.
Semantic Meaning: The ratio of inertial forces to viscous forces has
exploded. High information density in the current event window.

4. Viscous Resistance (The "Grind")

Price is "grinding against a wall."
Indicators: Low Viscosity (wide spreads/high resistance) with moderate
Divergence.
Semantic Meaning: The market is "thick" or viscous. Every tick move
requires massive traded volume.

# Summary of Fluid Categories

| Category   | Visc (Spread) | Dominant Metric            | Market "Feel"      |
|:-----------|:--------------|:---------------------------|:-------------------|
| Laminar    | High (Tight)  | None (Low Activity)        | Smooth/Consistent  |
| Turbulent  | Variable      | Turbulence / Vorticity     | Shattered/Fragile  |
| Inertial   | Moderate      | Reynolds / Divergence      | Direct/Heavy       |
| Viscous    | Low (Wide)    | Divergence (at walls)      | Resistant/Grinding |

Viscosity is the inverse of the spread; activity, displacement and turbulence
are derived inline against the pair's own median-scaled baselines.
*/
/*
Signal applies order-book fluid dynamics per symbol from ticker frames.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
}

/*
NewSignal constructs the fluid signal. It holds no per-pair state: each
measurement reads its pair's recent history straight from the tree, so the
mechanics of BTC and DOGE are each judged against their own recent behaviour.
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
Measure scores each ticker row against its pair's recent history and yields one
measurement per symbol in the frame.
*/
func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil {
			return
		}

		if datura.Peek[string](datapoint, "channel") != "ticker" {
			return
		}

		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")

			if symbol == "" {
				return
			}

			bid := datura.Peek[float64](datapoint, "data", rowIndex, "bid")
			ask := datura.Peek[float64](datapoint, "data", rowIndex, "ask")
			last := datura.Peek[float64](datapoint, "data", rowIndex, "last")
			volume := datura.Peek[float64](datapoint, "data", rowIndex, "volume")

			if last <= 0 || volume <= 0 || ask < bid {
				continue
			}

			metrics := signal.deriveMetrics(symbol, bid, ask, last, volume)
			distribution := classify(metrics)

			measurement := datura.Acquire("fluid", datura.APPJSON)
			measurement.WithRole("measurement")
			measurement.WithScope(symbol)
			errnie.Error(measurement.SetOrigin(string(logic.SourceFluid)))
			measurement.SetTimestamp(datapoint.Timestamp())

			measurement.MergeOutput("viscosity", metrics.viscosity)
			measurement.MergeOutput("reynolds", metrics.reynolds)
			measurement.MergeOutput("divergence", metrics.divergence)
			measurement.MergeOutput("turbulence", metrics.turbulence)

			for _, share := range distribution {
				measurement.MergeOutput(share.key, share.mass)
				measurement.MergeOutput(fmt.Sprintf("category.%d", logic.CategoryIndex(share.category)), share.mass)
			}

			measurement.Merge("volume", volume)
			measurement.Merge("last", last)
			measurement.Merge("spread", ask-bid)
			measurement.Merge("volumeDelta", metrics.volumeDelta)
			measurement.Merge("displacement", metrics.displacement)
			measurement.Merge("timestamp", datapoint.Timestamp())

			if !yield(measurement) {
				return
			}
		}
	}
}

/*
fluidMetrics holds the inline-derived mechanical quantities for one frame: the
median-scaled inputs that classify feeds on plus the raw inputs the next frame
replays.
*/
type fluidMetrics struct {
	viscosity    float64
	reynolds     float64
	divergence   float64
	turbulence   float64
	volumeDelta  float64
	displacement float64
}

/*
deriveMetrics rebuilds the pair's baselines from the tree and scores this frame
against them. Viscosity is the inverse of the spread relative to the pair's own
median spread; Reynolds scales the traded-volume "work" by displacement;
divergence is the median-scaled price displacement; turbulence is the dispersion
of recent displacements (vorticity proxy). Baselines exclude the current sample.
*/
func (signal *Signal) deriveMetrics(symbol string, bid, ask, last, volume float64) fluidMetrics {
	spreads, volumes, displacements, prevLast, prevVolume := signal.history(symbol)

	spread := ask - bid
	volumeDelta := math.Max(0, volume-prevVolume)

	displacement := 0.0

	if prevLast > 0 {
		displacement = math.Abs(math.Log(last / prevLast))
	}

	// Viscosity is high when the spread is tight versus the pair's median.
	viscosity := spreadTightness(spread, spreads)

	// Reynolds is the ratio of inertial work (median-scaled volume) to the
	// viscous resistance (1 + spread term), amplified by displacement.
	reynolds := statutil.ScaleByMedianOrUnity(volumeDelta, volumes) * (1 + displacement)

	// Divergence is the median-scaled price detachment at the touch.
	divergence := statutil.ScaleByMedianOrUnity(displacement, displacements)

	// Turbulence is the coefficient of variation of recent displacements: a
	// churning, shattered book swings widely around its own mean.
	turbulence := dispersion(displacements)

	return fluidMetrics{
		viscosity:    viscosity,
		reynolds:     reynolds,
		divergence:   divergence,
		turbulence:   turbulence,
		volumeDelta:  volumeDelta,
		displacement: displacement,
	}
}

/*
history reads the pair's prior measurements from the tree and rebuilds the
spread, volume-delta and displacement baselines plus the previous frame's raw
last price and volume, all scoped to this symbol. The baseline depth is not
fixed: it is the most recent priors spanning a wall-clock window derived from
the pair's own tick cadence (see windowDepth), so a pair ticking once a minute
and one ticking ten times a second are normalised to comparable history.
*/
func (signal *Signal) history(symbol string) (
	spreads, volumes, displacements []float64,
	prevLast, prevVolume float64,
) {
	if signal.tree == nil {
		return nil, nil, nil, 0, 0
	}

	query := datura.Acquire("fluid", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceFluid)))

	defer query.Release()

	prefix := fmt.Sprintf("measurement/%s/%s/", symbol, logic.SourceFluid)

	var (
		sprs, vols, disps []float64
		stamps            []float64
	)

	for prior := range signal.tree.Seek([]byte(prefix)) {
		sprs = append(sprs, datura.Peek[float64](prior, "spread"))
		vols = append(vols, datura.Peek[float64](prior, "volumeDelta"))
		disps = append(disps, datura.Peek[float64](prior, "displacement"))
		stamps = append(stamps, datura.Peek[float64](prior, "timestamp"))
		prevLast = datura.Peek[float64](prior, "last")
		prevVolume = datura.Peek[float64](prior, "volume")
	}

	keep := statutil.WindowDepth(stamps)
	spreads = statutil.Tail(sprs, keep)
	volumes = statutil.Tail(vols, keep)
	displacements = statutil.Tail(disps, keep)

	return spreads, volumes, displacements, prevLast, prevVolume
}

/*
categoryShare names a candidate mechanical state by its wire key and global
logic category, with that category's share of the total evidence (0..1).
*/
type categoryShare struct {
	key      string
	category logic.CategoryType
	mass     float64
}

/*
classify scores the four mechanical states from the derived metrics (see the
type comment) and returns them as a distribution: each category's mass,
normalised so the four sum to 1.0. The market is rarely purely one state, so the
mix is the signal rather than a single winner. When no state has evidence (a
flat baseline) every mass is zero.
*/
func classify(metrics fluidMetrics) []categoryShare {
	shares := []categoryShare{
		// Laminar Stability: high viscosity (tight spread) with low activity
		// (low displacement and turbulence).
		{"laminar", logic.CategoryLaminar, metrics.viscosity / (1 + metrics.divergence + metrics.turbulence)},
		// Turbulent Chaos: dominant turbulence and vorticity.
		{"turbulent", logic.CategoryTurbulent, metrics.turbulence * (1 + metrics.divergence)},
		// Inertial Displacement: high Reynolds carried on strong divergence.
		{"inertial", logic.CategoryInertial, metrics.reynolds * metrics.divergence},
		// Viscous Resistance: wide spread (low viscosity) grinding against
		// moderate divergence.
		{"viscous", logic.CategoryViscous, metrics.divergence / (1 + metrics.viscosity)},
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

func spreadTightness(spread float64, baseline []float64) float64 {
	median := statutil.Median(baseline)

	if median <= 0 || spread <= 0 {
		return 0
	}

	return math.Min(1, median/spread)
}

/*
dispersion returns the coefficient of variation of a series (standard deviation
over mean), a scale-free measure of how widely the values swing around their own
centre. An empty or zero-mean series has no dispersion.
*/
func dispersion(series []float64) float64 {
	if len(series) < 2 {
		return 0
	}

	mean, std := stat.MeanStdDev(series, nil)

	if mean <= 0 || math.IsNaN(std) {
		return 0
	}

	return std / mean
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
