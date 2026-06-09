package exhaust

import (
	
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric/adaptive"
	floatring "github.com/theapemachine/symm/ring"
	"github.com/theapemachine/symm/telemetry"
)

type System struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	bus          *internal.Bus
	signals      sync.Map
	gauge        *telemetry.Gauge
	feedback     *market.Feedback
	crossSection *crossSection
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	ctx, cancel := context.WithCancel(ctx)

	measurementsCapacity := viper.GetInt("signals.exhaust.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

	historyCapacity := viper.GetInt("signals.exhaust.history_capacity")

	if historyCapacity <= 0 {
		historyCapacity = 24
	}

	bus := internal.NewBus(
		ctx,
		pool,
		[]string{"measurements", "ui"},
		[]string{"raw"},
	)

	gauge, gaugeErr := telemetry.NewGauge(bus, logic.SourceExhaustion)

	if gaugeErr != nil {
		cancel()
		errnie.Error(gaugeErr)

		return nil
	}

	return &System{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		bus:          bus,
		signals:      sync.Map{},
		gauge:        gauge,
		crossSection: newCrossSection(historyCapacity),
	}
}

func (system *System) Tick() error {
	for {
		message, err := system.bus.Receive("raw")

		if errnie.Error(err) != nil || message == nil {
			continue
		}

		var (
			signal *Signal
			ok     bool
			warmed bool
		)

		switch message.Type {
		case "symbols":
			symbols, symbolOk := message.Value.([]string); if symbolOk { system.gauge.RegisterSymbols(symbols) }
			continue
		case "trades":
			var trade *krakenmarket.TradeUpdate

			if trade, ok = message.Value.(*krakenmarket.TradeUpdate); !ok {
				errnie.Error(errors.New("exhaust: invalid trade"), "exhaust: invalid trade")
				continue
			}

			signal = system.LoadSignal(logic.EntityTrade, trade.Symbol)

			if signal == nil {
				errnie.Error(errors.New("exhaust: symbol not found"), "exhaust: symbol not found")
				continue
			}

			warmed = signal.Record(trade)

		case "ticker":
			var ticker *krakenmarket.TickerUpdate

			if ticker, ok = message.Value.(*krakenmarket.TickerUpdate); !ok {
				errnie.Error(errors.New("exhaust: invalid ticker"), "exhaust: invalid ticker")
				continue
			}

			signal = system.LoadSignal(logic.EntityTick, ticker.Symbol)

			if signal == nil {
				errnie.Error(errors.New("exhaust: symbol not found"), "exhaust: symbol not found")
				continue
			}

			warmed = signal.Record(ticker)

		case "book":
			var book *krakenmarket.Book

			if book, ok = message.Value.(*krakenmarket.Book); !ok {
				errnie.Error(errors.New("exhaust: invalid book"), "exhaust: invalid book")
				continue
			}

			signal = system.LoadSignal(logic.EntityBook, book.Symbol)

			if signal == nil {
				errnie.Error(errors.New("exhaust: symbol not found"), "exhaust: symbol not found")
				continue
			}

			warmed = signal.Record(book)

		case "feedback":
			var feedback *market.Feedback

			if feedback, ok = message.Value.(*market.Feedback); !ok {
				errnie.Error(errors.New("exhaust: invalid feedback"), "exhaust: invalid feedback")
				continue
			}

			system.feedback = feedback
			continue
		}

		if signal == nil {
			continue
		}

		measurement, measureErr := signal.Measure(system.feedback)

		if errnie.Error(measureErr) != nil {
			continue
		}

		system.bus.Send(
			"measurements",
			"measurements",
			measurement,
		)

		errnie.Error(system.gauge.Publish(
			measurement,
			signal.symbol,
			warmed,
		))
	}
}

func (system *System) LoadSignal(entity logic.EntityType, symbol string) *Signal {
	var (
		raw    any
		signal *Signal
		ok     bool
	)

	threshold := math.Min(math.Max(viper.GetFloat64("signals.exhaust.surprise_threshold"), 1.0), 5.0)
	alpha := math.Min(math.Max(viper.GetFloat64("signals.exhaust.alpha"), 0.1), 1.0)

	measurementsCapacity := viper.GetInt("signals.exhaust.measurements_capacity")

	if measurementsCapacity <= 0 {
		measurementsCapacity = 64
	}

	mapKey := fmt.Sprintf("%d:%s", entity, symbol)

	raw, _ = system.signals.LoadOrStore(
		mapKey, NewSignal(
			symbol,
			logic.NewEntity(entity),
			measurementsCapacity,
			system.crossSection,
			threshold,
			alpha,
		),
	)

	if signal, ok = raw.(*Signal); !ok {
		errnie.Error(errors.New("exhaust: symbol is not a Signal"), "exhaust: symbol is not a Signal")
		return nil
	}

	return signal
}

func (system *System) Close() error {
	system.cancel()
	return system.bus.Close()
}

type crossSection struct {
	universe sync.Map
	capacity int
}

type featureState struct {
	bidDepths   floatring.FloatRing
	askDepths   floatring.FloatRing
	densities   floatring.FloatRing
	spreads     floatring.FloatRing
	pressures   floatring.FloatRing
	imbalances  floatring.FloatRing
	pressureEMA *adaptive.EMA
	lastPrice   float64
}

func newCrossSection(capacity int) *crossSection {
	return &crossSection{capacity: capacity}
}

func (crossSection *crossSection) ensure(symbol string) *featureState {
	raw, _ := crossSection.universe.LoadOrStore(symbol, &featureState{
		bidDepths:   floatring.NewFloatRing(crossSection.capacity),
		askDepths:   floatring.NewFloatRing(crossSection.capacity),
		densities:   floatring.NewFloatRing(crossSection.capacity),
		spreads:     floatring.NewFloatRing(crossSection.capacity),
		pressures:   floatring.NewFloatRing(crossSection.capacity),
		imbalances:  floatring.NewFloatRing(crossSection.capacity),
		pressureEMA: adaptive.NewEMA(0),
	})

	state, ok := raw.(*featureState)

	if !ok {
		return nil
	}

	return state
}

func (crossSection *crossSection) observeBook(symbol string, book *krakenmarket.Book) {
	if book == nil {
		return
	}

	folded := krakenmarket.Book{}
	folded.Fold(*book, 0)

	touchMid, touchSpread, depth, touchOK := folded.TouchQuote()

	if !touchOK || touchMid <= 0 {
		return
	}

	midPrice := touchMid / 2
	state := crossSection.ensure(symbol)

	if state == nil {
		return
	}

	bidDepth := crossSection.sideDepth(folded.Bids)
	askDepth := crossSection.sideDepth(folded.Asks)

	if bidDepth > 0 {
		state.bidDepths.Push(bidDepth)
	}

	if askDepth > 0 {
		state.askDepths.Push(askDepth)
	}

	if depth > 0 {
		state.densities.Push(depth)
	}

	spreadBPS := (touchSpread / midPrice) * 10000

	if spreadBPS > 0 {
		state.spreads.Push(spreadBPS)
	}

	imbalance, imbalanceOK := crossSection.level1Imbalance(folded.Bids, folded.Asks)

	if imbalanceOK {
		state.imbalances.Push(imbalance)
	}

	state.lastPrice = midPrice
}

func (crossSection *crossSection) observeTrade(symbol string, trade *krakenmarket.TradeUpdate) {
	if trade == nil {
		return
	}

	state := crossSection.ensure(symbol)

	if state == nil {
		return
	}

	sign := 0.0

	if trade.Side == "buy" {
		sign = 1
	}

	if trade.Side == "sell" {
		sign = -1
	}

	if sign == 0 {
		return
	}

	smoothed, err := state.pressureEMA.Next(0, sign)

	if errnie.Error(err) != nil {
		return
	}

	state.pressures.Push(smoothed)

	if trade.Price > 0 {
		state.lastPrice = trade.Price
	}
}

func (crossSection *crossSection) observeTick(symbol string, ticker *krakenmarket.TickerUpdate) {
	if ticker == nil {
		return
	}

	state := crossSection.ensure(symbol)

	if state == nil {
		return
	}

	mid := ticker.Last

	if mid <= 0 {
		mid = (ticker.Ask + ticker.Bid) / 2
	}

	spread := ticker.Ask - ticker.Bid

	if mid > 0 && spread > 0 {
		state.spreads.Push((spread / mid) * 10000)
	}

	if mid > 0 {
		state.lastPrice = mid
	}
}

func (crossSection *crossSection) snapshot(symbol string) (featureState, bool) {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return featureState{}, false
	}

	state, ok := raw.(*featureState)

	if !ok || state.lastPrice <= 0 {
		return featureState{}, false
	}

	return *state, true
}

func (crossSection *crossSection) sideDepth(levels []krakenmarket.BookLevel) float64 {
	depth := 0.0

	for _, level := range levels {
		depth += level.Qty
	}

	return depth
}

func (crossSection *crossSection) level1Imbalance(
	bids, asks []krakenmarket.BookLevel,
) (float64, bool) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	total := bids[0].Qty + asks[0].Qty

	if total <= 0 {
		return 0, false
	}

	return (bids[0].Qty - asks[0].Qty) / total, true
}
