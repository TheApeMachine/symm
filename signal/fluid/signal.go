package fluid

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
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
Signal applies order-book fluid dynamics per symbol from book, trades, and ticks.

Reynolds classifies laminar versus turbulent flow. Divergence is ∇·(ρv) at the
touch. Viscosity is replenishment resistance after consumption.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	fluidflow   *algorithm.Fluidflow
	classifier  *probability.Classifier
	transition  *probability.Transition
	registry    *Registry
	features    *Features
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
	classifier := probability.NewClassifier(
		fluidflow.LaminarReading(),
		fluidflow.TurbulentReading(),
		fluidflow.InertialReading(),
		fluidflow.ViscousReading(),
	)
	transition := probability.NewTransitionSurprise(
		5, 1.0/float64(viper.GetInt("signals.feed_ring_capacity")),
	)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		fluidflow:   fluidflow,
		classifier:  classifier,
		transition:  transition,
		registry:    registry,
	}

	bookFeed := feed.NewBook(ctx)
	bookFeed.OnUpdate = func(bookUpdate *krakenmarket.BookUpdate) {
		eventAt := bookUpdate.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		frame := *bookUpdate
		at := eventAt
		symbol := bookUpdate.Symbol

		registry.enqueue(symbol, func(state *FluidSymbol) {
			if err := state.FeedBook(frame, at); err != nil {
				errnie.Error(err)
			}
		})
	}

	tradeFeed := feed.NewTrade(ctx)
	tradeFeed.OnUpdate = func(tradeUpdate *krakenmarket.TradeUpdate) {
		if tradeUpdate.Price <= 0 || tradeUpdate.Qty <= 0 {
			return
		}

		eventAt := tradeUpdate.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		at := eventAt
		symbol := tradeUpdate.Symbol

		registry.enqueue(symbol, func(state *FluidSymbol) {
			if err := state.FeedTrade(at, tradeUpdate.Price, tradeUpdate.Qty, tradeUpdate.Side); err != nil {
				errnie.Error(err)
			}
		})
	}

	tickerFeed := feed.NewTicker(ctx)
	tickerFeed.OnUpdate = func(tickerUpdate *krakenmarket.TickerUpdate) {
		eventAt := tickerUpdate.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		frame := *tickerUpdate
		at := eventAt
		symbol := tickerUpdate.Symbol

		registry.enqueue(symbol, func(state *FluidSymbol) {
			if err := state.FeedTicker(frame, at); err != nil {
				errnie.Error(err)
			}
		})
	}

	signal.trade = tradeFeed
	signal.book = bookFeed
	signal.ticker = tickerFeed
	signal.features = NewFeatures(ctx, registry)
	signal.algo = nomagique.Number(
		fluidflow,
		classifier,
		transition,
	)

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "book":
		signal.book.Update(
			datura.As[krakenmarket.BookUpdates](artifact),
		)
	case "trade":
		signal.trade.Update(
			datura.As[krakenmarket.TradeUpdates](artifact),
		)
	case "ticker":
		signal.ticker.Update(
			datura.As[krakenmarket.TickerUpdates](artifact),
		)
	case "measurement":
		signal.Measure(artifact)
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := datura.Peek[string](in, "scope")

	signal.features.scope = scope
	signal.ticker.Scope = scope

	snapshot := signal.ticker.Snapshot(scope)
	features := signal.features.Artifact()

	if features == nil {
		return logic.Measurement{}, nil
	}

	if err := transport.NewFlipFlop(features, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := datura.Peek[float64](features, "fluidflow.strength")

	if strength <= 0 {
		return logic.Measurement{}, nil
	}

	categoryIndex := datura.Peek[int](features, "classifier.category")

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	confidence := datura.Peek[float64](features, "classifier.confidence")

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
		Surprise:   datura.Peek[float64](features, "transition.surprise"),
		ObservedAt: observedAt,
	}.UnlessPublishable(), nil
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

	if signal.registry != nil {
		signal.registry.Close()
	}

	return nil
}
