package fluid

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
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
	instrument  *broker.Instrument
	registry    *Registry
	ticker      *Ticker
	trade       *Trade
	book        *Book
	tickerCache *types.MarketFeed[kraken.TickerData]
	tradeCache  *types.MarketFeed[kraken.TradeData]
	bookCache   *types.MarketFeed[kraken.BookData]
	ui          chan []byte
}

/*
observeTicker retains replay data for direct signal tests. Live ingestion is owned
exclusively by trader.Market and does not register this method.
*/
func (signal *Signal) observeTicker(data []byte) {
	frame := utils.Unmarshal[kraken.Ticker](data)

	for _, row := range frame.Data {
		if err := signal.tickerCache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			errnie.Error(err)
			return
		}
	}
}

/*
observeTrade retains replay data for direct signal tests. Live ingestion is owned
exclusively by trader.Market and does not register this method.
*/
func (signal *Signal) observeTrade(data []byte) {
	frame := kraken.NewTrade(data)

	for _, row := range frame.Data {
		if err := signal.tradeCache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			errnie.Error(err)
			return
		}
	}
}

/*
observeBook retains enriched replay data for direct signal tests. Live ingestion is
owned exclusively by trader.Market and does not register this method.
*/
func (signal *Signal) observeBook(data []byte) {
	frame := kraken.NewBook(data)

	for _, row := range frame.Data {
		increment, err := signal.increment(row.Symbol)

		if err != nil {
			errnie.Error(err)
			return
		}

		row.PriceIncrement = increment

		if err := signal.bookCache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			errnie.Error(err)
			return
		}
	}
}

/*
increment resolves exchange tick size for direct replay. Central live book
ingestion performs this enrichment before producing the shared market cut.
*/
func (signal *Signal) increment(symbol string) (*decimal.Decimal, error) {
	if signal.instrument == nil {
		return decimal.NewFromInt64(0), nil
	}

	pair, err := signal.instrument.Pair(symbol)

	if err != nil {
		return nil, err
	}

	return pair.PriceIncrement.Copy(), nil
}

/*
Interest requires ticker, trade, and book streams; the mechanical metrics merge
all three inputs into one causal event timeline per symbol.
*/
func (signal *Signal) Interest() types.StreamInterest {
	return types.StreamAll
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	measurements, err := signal.Calculate(thesis.Market())

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return measurements
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.WireMeasurements(measurements),
	}.Marshal():
	default:
	}
}

func NewSignal(
	ctx context.Context, api *websocket.API, instrument *broker.Instrument, ui chan []byte,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	registry := NewSyncRegistry()

	signal := &Signal{
		ctx:        ctx,
		cancel:     cancel,
		instrument: instrument,
		registry:   registry,
		ui:         ui,
		ticker:     NewTicker(registry),
		trade:      NewTrade(registry),
		book:       NewBook(registry),
	}

	return signal
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
