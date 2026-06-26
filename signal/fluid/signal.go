package fluid

import (
	"context"
	"encoding/json"
	"iter"
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
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
type Signal struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	tree     *dmt.Tree
	registry *Registry
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:      ctx,
		cancel:   cancel,
		tree:     tree,
		registry: NewSyncRegistry(),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"book", "trade", "ticker"}
}

/*
Measure feeds each scoped ticker, book, or trade row through the per-symbol
fluid solver and yields a measurement only after the book lattice has integrated.
*/
func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil {
			return
		}

		channel := datura.Peek[string](datapoint, "channel")

		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")

			if symbol == "" {
				return
			}

			state := signal.registry.loadSymbol(symbol)

			if state == nil {
				continue
			}

			eventAt := eventTime(datapoint, rowIndex)

			switch channel {
			case "ticker":
				if err := state.FeedTicker(tickerUpdate(datapoint, rowIndex, symbol), eventAt); errnie.Error(err) != nil {
					continue
				}
			case "book":
				update := bookUpdate(datapoint, rowIndex, symbol, eventAt)

				if len(update.Bids) == 0 && len(update.Asks) == 0 {
					continue
				}

				signal.setInstrumentTick(symbol)

				if !state.HasBook() && len(update.Bids) > 0 && len(update.Asks) > 0 {
					update.Type = "snapshot"
				}

				if err := state.FeedBook(update, eventAt); errnie.Error(err) != nil {
					continue
				}
			case "trade":
				trade := tradeUpdate(datapoint, rowIndex, symbol, eventAt)

				if trade.Price <= 0 || trade.Qty <= 0 {
					continue
				}

				if err := state.FeedTrade(eventAt, trade.Price, trade.Qty, trade.Side); errnie.Error(err) != nil {
					continue
				}

				continue
			default:
				return
			}

			reading, ok := state.Reading()

			if !ok {
				continue
			}

			measurement := measurementFromReading(reading, eventAt)

			if measurement == nil {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func eventTime(datapoint *datura.Artifact, rowIndex int) time.Time {
	stamp := datura.Peek[string](datapoint, "data", rowIndex, "timestamp")

	if stamp != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			return parsed.UTC()
		}
	}

	if datapoint != nil && datapoint.Timestamp() > 0 {
		return time.Unix(0, datapoint.Timestamp()).UTC()
	}

	return time.Now().UTC()
}

func tickerUpdate(datapoint *datura.Artifact, rowIndex int, symbol string) TickerUpdate {
	return TickerUpdate{
		Symbol:    symbol,
		Last:      datura.Peek[float64](datapoint, "data", rowIndex, "last"),
		Bid:       datura.Peek[float64](datapoint, "data", rowIndex, "bid"),
		Ask:       datura.Peek[float64](datapoint, "data", rowIndex, "ask"),
		BidQty:    datura.Peek[float64](datapoint, "data", rowIndex, "bid_qty"),
		AskQty:    datura.Peek[float64](datapoint, "data", rowIndex, "ask_qty"),
		Change:    datura.Peek[float64](datapoint, "data", rowIndex, "change"),
		ChangePct: datura.Peek[float64](datapoint, "data", rowIndex, "change_pct"),
		High:      datura.Peek[float64](datapoint, "data", rowIndex, "high"),
		Low:       datura.Peek[float64](datapoint, "data", rowIndex, "low"),
		Volume:    datura.Peek[float64](datapoint, "data", rowIndex, "volume"),
		Timestamp: eventTime(datapoint, rowIndex),
	}
}

func bookUpdate(datapoint *datura.Artifact, rowIndex int, symbol string, eventAt time.Time) BookUpdate {
	updateType := datura.Peek[string](datapoint, "data", rowIndex, "type")

	if updateType == "" {
		updateType = datura.Peek[string](datapoint, "type")
	}

	return BookUpdate{
		Symbol:    symbol,
		Type:      updateType,
		Timestamp: eventAt,
		Bids:      bookLevels(datapoint, rowIndex, "bids"),
		Asks:      bookLevels(datapoint, rowIndex, "asks"),
	}
}

func bookLevels(datapoint *datura.Artifact, rowIndex int, side string) []BookLevel {
	levels := []BookLevel{}

	for levelIndex := 0; ; levelIndex++ {
		price := datura.Peek[float64](datapoint, "data", rowIndex, side, levelIndex, "price")

		if price <= 0 {
			return levels
		}

		levels = append(levels, BookLevel{
			Price: price,
			Qty:   datura.Peek[float64](datapoint, "data", rowIndex, side, levelIndex, "qty"),
		})
	}
}

func tradeUpdate(datapoint *datura.Artifact, rowIndex int, symbol string, eventAt time.Time) TradeUpdate {
	return TradeUpdate{
		Symbol:    symbol,
		Side:      datura.Peek[string](datapoint, "data", rowIndex, "side"),
		Price:     datura.Peek[float64](datapoint, "data", rowIndex, "price"),
		Qty:       datura.Peek[float64](datapoint, "data", rowIndex, "qty"),
		Timestamp: eventAt,
	}
}

func measurementFromReading(reading fluidReading, eventAt time.Time) *datura.Artifact {
	measurement := datura.Acquire("fluid", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(reading.symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceFluid)))
	measurement.SetTimestamp(eventAt.UnixNano())

	measurement.MergeOutput("viscosity", reading.viscosity)
	measurement.MergeOutput("reynolds", reading.reynolds)
	measurement.MergeOutput("divergence", reading.divergence)
	measurement.MergeOutput("vorticity", reading.vorticity)
	measurement.MergeOutput("turbulence", reading.turbulence)
	measurement.MergeOutput("sourceBalance", reading.sourceBalance)
	measurement.MergeOutput("memory", reading.memory)
	measurement.MergeOutput("midAddRate", reading.midAddRate)
	measurement.MergeOutput("midExecuteRate", reading.midExecuteRate)

	confidence := dist.Write(measurement, classify(reading))

	if confidence <= 0 {
		measurement.Release()

		return nil
	}

	measurement.Merge("price", reading.price)
	measurement.Merge("last", reading.price)
	measurement.Merge("spreadBPS", reading.spreadBPS)
	measurement.Merge("volume", reading.volume)
	measurement.Merge("change_pct", reading.changePct)
	measurement.Merge("re", reading.reynolds)
	measurement.Merge("div", reading.divergence)
	measurement.Merge("vort", reading.vorticity)
	measurement.Merge("turb", reading.turbulence)
	measurement.Merge("visc", reading.viscosity)
	measurement.Merge("src_bal", reading.sourceBalance)
	measurement.Merge("memory", reading.memory)
	measurement.Merge("midAddRate", reading.midAddRate)
	measurement.Merge("midExecuteRate", reading.midExecuteRate)
	measurement.Merge("timestamp", eventAt.UnixNano())

	return measurement
}

func (signal *Signal) setInstrumentTick(symbol string) {
	if signal.tree == nil {
		return
	}

	raw, ok := signal.tree.Get([]byte("instrument/" + symbol + "/"))

	if !ok {
		return
	}

	artifact := datura.Acquire("fluid", datura.APPJSON)
	defer artifact.Release()

	if _, err := artifact.Unpack(raw); err != nil {
		return
	}

	var meta struct {
		TickSize float64 `json:"tick_size"`
	}

	if json.Unmarshal(artifact.DecryptPayload(), &meta) != nil {
		return
	}

	signal.registry.SetInstrumentTickSize(symbol, meta.TickSize)
}

func classify(reading fluidReading) []dist.Share {
	viscosity := medianScale(reading.viscosity, reading.dynamics.viscosityHistory)
	reynolds := medianScale(reading.reynolds, reading.dynamics.reynoldsHistory)
	divergence := medianScale(math.Abs(reading.divergence), reading.dynamics.divergenceHistory)
	vorticity := medianScale(math.Abs(reading.vorticity), reading.dynamics.vorticityHistory)
	turbulence := medianScale(math.Abs(reading.turbulence), reading.dynamics.turbulenceHistory)
	activity := divergence + vorticity + turbulence

	return []dist.Share{
		{Key: "laminar", Category: logic.CategoryLaminar, Mass: positive(viscosity / (1 + activity))},
		{Key: "turbulent", Category: logic.CategoryTurbulent, Mass: positive(turbulence + vorticity)},
		{Key: "inertial", Category: logic.CategoryInertial, Mass: positive(reynolds * divergence)},
		{Key: "viscous", Category: logic.CategoryViscous, Mass: positive((divergence + reading.memory) / (1 + viscosity))},
	}
}

func medianScale(sample float64, baseline []float64) float64 {
	if sample < 0 || math.IsNaN(sample) || math.IsInf(sample, 0) {
		return 0
	}

	if len(baseline) == 0 {
		if sample <= 0 {
			return 0
		}

		return 1
	}

	median := sampleQuantile(0.5, baseline)

	if median <= 0 {
		if sample <= 0 {
			return 0
		}

		return 1
	}

	return sample / median
}

func positive(value float64) float64 {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return value
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	if signal.registry != nil {
		signal.registry.Close()
	}

	return errnie.Error(err)
}
