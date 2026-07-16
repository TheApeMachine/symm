package fluid

import (
	"container/ring"
	"context"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Fluid is the mechanical perspective on the order book, mapping
microstructural metrics — Reynolds Number (Re), Divergence (Div),
Vorticity (Vort), and Turbulence (Turb) — against Viscosity (Visc).

1. What it measures exactly (in isolation)

The Fluid signal applies order-book fluid dynamics per symbol from book,
trades, and ticks. Reynolds distinguishes laminar from turbulent flow.
Divergence is ∇·(ρv) at the touch. Viscosity is replenishment resistance
after consumption.

It exposes evidence for the following mechanical states without selecting one
inside the signal:

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

# Summary of Fluid Evidence

| State      | Visc (Spread) | Dominant Metric            | Market "Feel"      |
|:-----------|:--------------|:---------------------------|:-------------------|
| Laminar    | High (Tight)  | None (Low Activity)        | Smooth/Consistent  |
| Turbulent  | Variable      | Turbulence / Vorticity     | Shattered/Fragile  |
| Inertial   | Moderate      | Reynolds / Divergence      | Direct/Heavy       |
| Viscous    | Low (Wide)    | Divergence (at walls)      | Resistant/Grinding |

Viscosity is the inverse of the spread; activity, displacement and turbulence
are derived inline against the pair's own median-scaled baselines.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	api         *websocket.API
	instrument  *broker.Instrument
	registry    *Registry
	ticker      *Ticker
	trade       *Trade
	book        *Book
	tickerCache *sync.Map
	tradeCache  *sync.Map
	bookCache   *sync.Map
}

func NewSignal(
	ctx context.Context, api *websocket.API, instrument *broker.Instrument,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	registry := NewSyncRegistry()

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		api:         api,
		instrument:  instrument,
		registry:    registry,
		ticker:      NewTicker(registry),
		trade:       NewTrade(registry),
		book:        NewBook(registry),
		tickerCache: &sync.Map{},
		tradeCache:  &sync.Map{},
		bookCache:   &sync.Map{},
	}

	signal.api.On("ticker", signal.onTicker)
	signal.api.On("trade", signal.onTrade)
	signal.api.On("book", signal.onBook)

	return signal
}

/*
onTicker decodes ticker updates and feeds symbol state so price context stays
current.
*/
func (signal *Signal) onTicker(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := utils.Unmarshal[kraken.Ticker](data)

	if len(frame.Data) == 0 {
		return
	}

	for _, data := range frame.Data {
		found, _ := signal.tickerCache.LoadOrStore(data.Symbol, ring.New(
			viper.GetInt("signals.feed_ring_capacity"),
		))

		track := found.(*ring.Ring)
		track.Value = data
		signal.tickerCache.Store(data.Symbol, track.Next())
	}
}

/*
onTrade decodes trade updates and feeds executed flow so tape activity reaches
the grid.
*/
func (signal *Signal) onTrade(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewTrade(data)

	if len(frame.Data) == 0 {
		return
	}

	for _, data := range frame.Data {
		found, _ := signal.tradeCache.LoadOrStore(data.Symbol, ring.New(
			viper.GetInt("signals.feed_ring_capacity"),
		))

		track := found.(*ring.Ring)
		track.Value = data
		signal.tradeCache.Store(data.Symbol, track.Next())
	}
}

/*
onBook decodes book updates and feeds resting-liquidity changes so the grid
follows the live book.
*/
func (signal *Signal) onBook(data []byte) {
	if len(data) == 0 {
		return
	}

	frame := kraken.NewBook(data)

	if len(frame.Data) == 0 {
		return
	}

	for index := range frame.Data {
		frame.Data[index].PriceIncrement = signal.increment(frame.Data[index].Symbol)
	}

	for _, data := range frame.Data {
		found, _ := signal.bookCache.LoadOrStore(data.Symbol, ring.New(
			viper.GetInt("signals.feed_ring_capacity"),
		))

		track := found.(*ring.Ring)
		track.Value = data
		signal.bookCache.Store(data.Symbol, track.Next())
	}
}

/*
increment resolves the exchange price increment for symbol, defaulting to a
zero decimal (skipped downstream by Book.Measure's own guard) until
instrument metadata for that symbol has arrived.
*/
func (signal *Signal) increment(symbol string) decimal.Decimal {
	if signal.instrument == nil {
		// ponytail: instrument metadata may arrive after the first book frame;
		// zero increment keeps the row skippable until Pair resolves.
		return *decimal.NewFromInt64(0)
	}

	pair, err := signal.instrument.Pair(symbol)

	if err != nil {
		// ponytail: unknown symbols stay at zero increment until the universe
		// registers the pair and subsequent frames carry a real tick size.
		return *decimal.NewFromInt64(0)
	}

	return pair.Increment()
}

/*
Measure feeds every ticker, trade, and book row cached since the last tick
through the per-symbol fluid solver, in that order: ticker and trade rows
only update per-symbol state (FluidSymbol.Reading has nothing to read yet
after them), while book rows are what actually measure and emit, so book
runs last against the freshest state.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	tickers := make([]kraken.TickerData, 0)
	signal.tickerCache.Range(func(key, value any) bool {
		value.(*ring.Ring).Do(func(value any) {
			if value != nil {
				tickers = append(tickers, value.(kraken.TickerData))
			}
		})
		signal.tickerCache.Delete(key)

		return true
	})

	trades := make([]kraken.TradeData, 0)
	signal.tradeCache.Range(func(key, value any) bool {
		value.(*ring.Ring).Do(func(value any) {
			if value != nil {
				trades = append(trades, value.(kraken.TradeData))
			}
		})
		signal.tradeCache.Delete(key)

		return true
	})

	books := make([]kraken.BookData, 0)
	signal.bookCache.Range(func(key, value any) bool {
		value.(*ring.Ring).Do(func(value any) {
			if value != nil {
				books = append(books, value.(kraken.BookData))
			}
		})
		signal.bookCache.Delete(key)

		return true
	})

	out := make([]*types.Measurement, 0, len(tickers)+len(trades)+len(books))

	appendMeasurements(&out, tickers, signal.ticker.Measure)
	appendMeasurements(&out, trades, signal.trade.Measure)
	appendMeasurements(&out, books, signal.book.Measure)

	thesis.Measurements = append(thesis.Measurements, out...)

	return thesis
}

func appendMeasurements[Row any](
	out *[]*types.Measurement,
	rows []Row,
	measure func(Row) ([]*types.Measurement, error),
) {
	for _, row := range rows {
		measurements, err := measure(row)

		if err != nil {
			errnie.Error(err)
			continue
		}

		*out = append(*out, measurements...)
	}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
