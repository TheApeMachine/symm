package fluid

import (
	"context"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	feed "github.com/theapemachine/symm/signal"
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
resistance) but moderate Divergence and high Memory (preserved via
fractional differencing).

---

2. Semantically, what story does it tell?

The Fluid signal tells the story of mechanical health — whether the
"vapour pipe" of the market is running smoothly or shattering.

The "Smooth Pipe" Story: Price moves are smooth and the book absorbs
updates without churning. The market is at a constant, manageable diameter.

The "Shattered Mechanics" Story: The fractional differencing filter detects
that the series is becoming non-stationary and losing its "memory" of
previous levels. This is genuine microstructural chaos, not just price
volatility.

The "Grind" Story: Every tick move requires a massive amount of "work"
(traded volume), but the signal remembers that price has been exhausted at
this level for a long duration.

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
volatility. The series is becoming non-stationary.

3. Inertial Displacement (Directional Surge)

The market is being forcibly "pushed" by one-sided order flow.
Indicators: A high Reynolds Number and high Divergence.
Semantic Meaning: The ratio of inertial forces to viscous forces has
exploded. Massive information density within a single volume-clocked bar.

4. Viscous Resistance (The "Grind")

Price is "grinding against a wall."
Indicators: Low Viscosity (wide spreads/high resistance) but moderate
Divergence and high Memory.
Semantic Meaning: The market is "thick" or viscous. Every tick move
requires massive traded volume.

# Summary of Fluid Categories

| Category   | Visc (Spread) | Dominant Metric            | Market "Feel"      |
|:-----------|:--------------|:---------------------------|:-------------------|
| Laminar    | High (Tight)  | None (Low Activity)        | Smooth/Consistent  |
| Turbulent  | Variable      | Turbulence / Vorticity     | Shattered/Fragile  |
| Inertial   | Moderate      | Reynolds / Divergence      | Direct/Heavy       |
| Viscous    | Low (Wide)    | Divergence (at walls)      | Resistant/Grinding |

Field Activity takes the maximum absolute value of the four fluid dynamics,
and Viscosity is the inverse of the spread.
*/
/*
Signal applies order-book fluid dynamics per symbol from book, trades, and ticks.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	tree        *dmt.Tree
	registry    *Registry
	trade       *feed.Trade
	book        *feed.Book
	ticker      *feed.Ticker
}

/*
NewSignal composes the fluid-flow pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	registry := NewRegistry(ctx)
	fluidflow := algorithm.NewFluidflow()

	bookFeed := feed.NewBook(ctx)
	tradeFeed := feed.NewTrade(ctx)
	tickerFeed := feed.NewTicker(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		registry:    registry,
		tree:        dmt.NewTree(""),
		trade:       tradeFeed,
		book:        bookFeed,
		ticker:      tickerFeed,
		algo: nomagique.Number(
			fluidflow,
			probability.NewClassifier(
				fluidflow.LaminarReading(),
				fluidflow.TurbulentReading(),
				fluidflow.InertialReading(),
				fluidflow.ViscousReading(),
			),
		),
	}

	bookFeed.OnUpdate = func(symbol string, element []byte) {
		if symbol == "" || len(element) == 0 {
			return
		}

		eventAt, eventOK := feed.ElementTime(element, "timestamp")

		if !eventOK {
			eventAt = time.Now()
		}

		frame := bookElementToKraken(symbol, element, eventAt)
		at := eventAt

		registry.enqueue(symbol, func(state *FluidSymbol) {
			if err := state.FeedBook(frame, at); err != nil {
				errnie.Error(err)
			}

			signal.publishFeatures(symbol)
		})
	}

	tradeFeed.OnUpdate = func(symbol string, element []byte) {
		price, priceOK := feed.PeekElementOK[float64](element, "price")
		qty, qtyOK := feed.PeekElementOK[float64](element, "qty")

		if symbol == "" || !priceOK || !qtyOK || price <= 0 || qty <= 0 {
			return
		}

		eventAt, eventOK := feed.ElementTime(element, "timestamp")

		if !eventOK {
			eventAt = time.Now()
		}

		side, _ := feed.PeekElementOK[string](element, "side")
		at := eventAt

		registry.enqueue(symbol, func(state *FluidSymbol) {
			if err := state.FeedTrade(at, price, qty, side); err != nil {
				errnie.Error(err)
			}

			signal.publishFeatures(symbol)
		})
	}

	tickerFeed.OnUpdate = func(symbol string, element []byte) {
		if symbol == "" || len(element) == 0 {
			return
		}

		eventAt, eventOK := feed.ElementTime(element, "timestamp")

		if !eventOK {
			eventAt = time.Now()
		}

		frame := tickerElementToKraken(symbol, element, eventAt)
		at := eventAt

		registry.enqueue(symbol, func(state *FluidSymbol) {
			if err := state.FeedTicker(frame, at); err != nil {
				errnie.Error(err)
			}

			signal.publishFeatures(symbol)
		})
	}

	return signal
}

/*
SetInstrumentTickSize records the exchange price increment for one symbol.
*/
func (signal *Signal) SetInstrumentTickSize(symbol string, priceIncrement float64) {
	if signal == nil || signal.registry == nil {
		return
	}

	signal.registry.SetInstrumentTickSize(symbol, priceIncrement)
}

func bookElementToKraken(symbol string, element []byte, eventAt time.Time) krakenmarket.BookUpdate {
	update := krakenmarket.BookUpdate{
		Symbol:    symbol,
		Timestamp: eventAt,
	}

	if feed.UnmarshalElement(element, &update) != nil {
		update.Symbol = symbol
		update.Timestamp = eventAt

		feed.EachBookLevelElement(element, "bids", func(price float64, qty float64) {
			update.Bids = append(update.Bids, krakenmarket.BookLevel{Price: price, Qty: qty})
		})

		feed.EachBookLevelElement(element, "asks", func(price float64, qty float64) {
			update.Asks = append(update.Asks, krakenmarket.BookLevel{Price: price, Qty: qty})
		})
	}

	if update.Symbol == "" {
		update.Symbol = symbol
	}

	if update.Timestamp.IsZero() {
		update.Timestamp = eventAt
	}

	if update.Type == "" {
		update.Type = "update"
	}

	return update
}

func tickerElementToKraken(symbol string, element []byte, eventAt time.Time) krakenmarket.TickerUpdate {
	update := krakenmarket.TickerUpdate{Symbol: symbol, Timestamp: eventAt}
	_ = feed.UnmarshalElement(element, &update)
	update.Symbol = symbol
	update.Timestamp = eventAt

	return update
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "book":
		signal.book.Update(artifact)
	case "trade":
		signal.trade.Update(artifact)
	case "ticker":
		signal.ticker.Update(artifact)
	case "measurement":
		signal.Measure(*artifact)
	}

	return nil
}

func (signal *Signal) Measure(query datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	signal.publishFeatures(scope)

	var measurement *datura.Artifact

	prefix := "features/" + scope

	for inbound := range signal.tree.Seek([]byte(prefix)) {
		processed := datura.Acquire("fluid", datura.APPJSON)

		if processed == nil {
			continue
		}

		payload, payloadOK := feed.ArtifactPayload(inbound)

		if !payloadOK {
			processed.Release()
			continue
		}

		if !feed.ValidFloatPayload(payload, feed.FluidflowMinFloats) {
			processed.Release()
			continue
		}

		if processed.WithPayload(payload) == nil {
			processed.Release()
			continue
		}

		if flipErr := transport.NewFlipFlop(processed, signal.algo); flipErr != nil {
			_ = processed.WithError(flipErr)
		}

		if datura.Peek[int](processed, "classifier.category") <= 0 {
			processed.Release()
			continue
		}

		if datura.Peek[float64](processed, "classifier.confidence") <= 0 {
			processed.Release()
			continue
		}

		processed.WithRole("measurement")
		processed.WithScope(scope)

		measurement = processed
	}

	if measurement != nil {
		feed.InsertMeasurement(signal.tree, measurement)
	}

	return measurement
}

func (signal *Signal) publishFeatures(scope string) {
	artifact := signal.featureArtifact(scope)

	if artifact == nil || signal.tree == nil {
		return
	}

	feed.InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	state := signal.registry.loadSymbol(scope)

	if state == nil {
		return nil
	}

	reading, ok := state.Reading()

	if !ok {
		return nil
	}

	turbulentFloor, turbulentReady := reading.dynamics.turbulentReynoldsFloor()
	icebergScore := reading.dynamics.icebergScore(reading.midAddRate, reading.midExecuteRate)

	turbulentReadyFlag := 0.0

	if turbulentReady {
		turbulentReadyFlag = 1
	}

	changePct := state.changePct

	if changePct <= 0 && reading.spreadBPS > 0 {
		changePct = reading.spreadBPS / 10000
	}

	artifact := datura.Acquire("fluid-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(feed.EncodePayload(
		reading.reynolds,
		math.Abs(reading.divergence),
		reading.viscosity,
		reading.midAddRate,
		reading.midExecuteRate,
		reading.dynamics.laminarReynoldsCeiling(reading.reynolds),
		turbulentFloor,
		turbulentReadyFlag,
		reading.dynamics.laminarDivergenceEdge(),
		icebergScore,
		reading.price,
		reading.spreadBPS,
		changePct,
		state.volume,
	))

	return artifact
}

func fluidCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryLaminar
	case 2:
		return logic.CategoryTurbulent
	case 3:
		return logic.CategoryInertial
	case 4:
		return logic.CategoryViscous
	default:
		return logic.CategoryTypeNone
	}
}

/*
FieldSnapshot builds the fluid dashboard payload from the live registry rows.
*/
func (signal *Signal) FieldSnapshot(eventAt time.Time) (map[string]any, error) {
	if signal == nil || signal.registry == nil {
		return nil, nil
	}

	if eventAt.IsZero() {
		return nil, fmt.Errorf("fluid: field snapshot event time is zero")
	}

	symbols := make([]map[string]any, 0, 64)

	signal.registry.RangeRows(eventAt, func(row map[string]any) bool {
		symbols = append(symbols, row)

		return true
	})

	if len(symbols) == 0 {
		return nil, nil
	}

	return map[string]any{
		"type":         "fluid",
		"ts":           eventAt.UTC().Format(time.RFC3339Nano),
		"symbol_count": len(symbols),
		"symbols":      symbols,
	}, nil
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	if signal.registry != nil {
		signal.registry.Close()
	}

	return err
}
