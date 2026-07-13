package fluid

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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
resistance) with moderate Divergence. Span-normalized recent price memory
reinforces viscous scoring when replenishment lags displacement.

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
type Signal[T any] struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	registry   *Registry
	fluidflow  *equation.Fluidflow
	classifier *probability.ScoreClassifier
	ticker     *Ticker
	trade      *Trade
	book       *Book
}

func NewSignal[T any](ctx context.Context) *Signal[T] {
	ctx, cancel := context.WithCancel(ctx)
	registry := NewSyncRegistry()

	return &Signal[T]{
		ctx:       ctx,
		cancel:    cancel,
		registry:  registry,
		fluidflow: equation.NewFluidflow(),
		classifier: probability.NewScoreClassifier(
			[]string{"laminarScore", "turbulentScore", "inertialScore", "viscousScore"},
			[]float64{
				float64(types.CategoryIndex(types.CategoryLaminar)),
				float64(types.CategoryIndex(types.CategoryTurbulent)),
				float64(types.CategoryIndex(types.CategoryInertial)),
				float64(types.CategoryIndex(types.CategoryViscous)),
			},
		),
		ticker: NewTicker(registry),
		trade:  NewTrade(registry),
		book:   NewBook(registry),
	}
}

/*
Measure feeds each scoped ticker, book, or trade row through the per-symbol
fluid solver and yields a measurement only after the book lattice has integrated.
*/
func (signal *Signal[T]) Measure(
	input T,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	switch row := any(input).(type) {
	case kraken.TickerData:
		return signal.ticker.Measure(row)
	case kraken.TradeData:
		return signal.trade.Measure(row)
	case kraken.BookData:
		return signal.book.Measure(row)
	}

	return nil, nil
}

func (signal *Signal[T]) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal[T]) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
