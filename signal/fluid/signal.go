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
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	surpriseTree *dmt.Tree
	registry     *Registry
	measureScope string
	trade        *feed.Trade
	book         *feed.Book
	ticker       *feed.Ticker
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
	surpriseTree, _ := dmt.NewTree("")

	bookFeed := feed.NewBook(ctx)
	bookFeed.OnUpdate = func(bookRecord *feed.BookRecord) {
		if bookRecord == nil {
			return
		}

		eventAt := bookRecord.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		frame := bookRecordToKraken(bookRecord)
		at := eventAt
		symbol := bookRecord.Symbol

		registry.enqueue(symbol, func(state *FluidSymbol) {
			if err := state.FeedBook(frame, at); err != nil {
				errnie.Error(err)
			}
		})
	}

	tradeFeed := feed.NewTrade(ctx)
	tradeFeed.OnUpdate = func(tradeRecord *feed.TradeRecord) {
		if tradeRecord == nil || tradeRecord.Price <= 0 || tradeRecord.Qty <= 0 {
			return
		}

		eventAt := tradeRecord.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		at := eventAt
		symbol := tradeRecord.Symbol

		registry.enqueue(symbol, func(state *FluidSymbol) {
			if err := state.FeedTrade(at, tradeRecord.Price, tradeRecord.Qty, tradeRecord.Side); err != nil {
				errnie.Error(err)
			}
		})
	}

	tickerFeed := feed.NewTicker(ctx)
	tickerFeed.OnUpdate = func(tickerRecord *feed.TickerRecord) {
		if tickerRecord == nil {
			return
		}

		eventAt := tickerRecord.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		frame := tickerRecordToKraken(tickerRecord)
		at := eventAt
		symbol := tickerRecord.Symbol

		registry.enqueue(symbol, func(state *FluidSymbol) {
			if err := state.FeedTicker(frame, at); err != nil {
				errnie.Error(err)
			}
		})
	}

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		surpriseTree: surpriseTree,
		registry:     registry,
		trade:        tradeFeed,
		book:         bookFeed,
		ticker:       tickerFeed,
		algo: nomagique.Number(
			fluidflow,
			probability.NewClassifier(
				fluidflow.LaminarReading(),
				fluidflow.TurbulentReading(),
				fluidflow.InertialReading(),
				fluidflow.ViscousReading(),
			),
			probability.NewDMTSurprise(
				surpriseTree,
				5,
			),
		),
	}

	return signal
}

func bookRecordToKraken(record *feed.BookRecord) krakenmarket.BookUpdate {
	update := krakenmarket.BookUpdate{
		Symbol:    record.Symbol,
		Type:      "snapshot",
		Timestamp: record.Timestamp,
	}

	for _, bid := range record.Bids {
		update.Bids = append(update.Bids, krakenmarket.BookLevel{
			Price: bid.Price,
			Qty:   bid.Qty,
		})
	}

	for _, ask := range record.Asks {
		update.Asks = append(update.Asks, krakenmarket.BookLevel{
			Price: ask.Price,
			Qty:   ask.Qty,
		})
	}

	return update
}

func tickerRecordToKraken(record *feed.TickerRecord) krakenmarket.TickerUpdate {
	return krakenmarket.TickerUpdate{
		Symbol:    record.Symbol,
		Ask:       record.Ask,
		AskQty:    record.AskQty,
		Bid:       record.Bid,
		BidQty:    record.BidQty,
		Change:    record.Change,
		ChangePct: record.ChangePct,
		High:      record.High,
		Last:      record.Last,
		Low:       record.Low,
		Volume:    record.Volume,
		VWAP:      record.VWAP,
		Timestamp: record.Timestamp,
	}
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
		signal.Measure(artifact)
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := datura.Peek[string](in, "scope")
	signal.measureScope = scope
	signal.trade.Scope = scope
	signal.book.Scope = scope
	signal.ticker.Scope = scope
	signal.trade.ResetReadHead()
	signal.book.ResetReadHead()
	signal.ticker.ResetReadHead()

	out := datura.Acquire("fluid-out", datura.Artifact_Type_json).WithScope(scope)

	if out == nil {
		return logic.Measurement{}, nil
	}

	errnie.Does(func() (int64, error) {
		return transport.Copy(
			signal.algo,
			io.MultiReader(signal.trade, signal.book, signal.ticker, signal),
		)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.IO, "failed to copy to algo", err,
		))
	})

	if err := transport.NewFlipFlop(out, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	snapshot := signal.ticker.Snapshot(scope)
	strength := datura.Peek[float64](out, "fluidflow.strength")

	if strength <= 0 {
		return logic.Measurement{}, nil
	}

	categoryIndex := datura.Peek[int](out, "classifier.category")

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	confidence := datura.Peek[float64](out, "classifier.confidence")

	if !logic.ScalarFinite(confidence) || confidence <= 0 {
		return logic.Measurement{}, nil
	}

	observedAt := snapshot.Observed

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	return logic.Measurement{
		Source:     logic.SourceFluid,
		Symbol:     scope,
		Price:      snapshot.Last,
		Strength:   strength,
		Volume:     snapshot.Volume,
		Spread:     signal.book.Spread(scope),
		Elapsed:    snapshot.Elapsed,
		Category:   fluidCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](out, "transition.surprise"),
		ObservedAt: observedAt,
	}.UnlessPublishable(), nil
}

func (signal *Signal) Read(buffer []byte) (int, error) {
	artifact := signal.featureArtifact(signal.measureScope)

	if artifact == nil {
		return 0, io.EOF
	}

	return feed.ReadFeatureArtifact(buffer, artifact)
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

func (signal *Signal) Close() error {
	signal.cancel()

	if signal.surpriseTree != nil {
		_ = signal.surpriseTree.Close()
	}

	if signal.registry != nil {
		signal.registry.Close()
	}

	return nil
}
