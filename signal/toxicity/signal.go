package toxicity

import (
	"context"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/websocket"
	signalshared "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal tracks whether near-touch liquidity is sincere, retreating, or bluffing
from Level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	previousTouch map[string]float64
	previousPrice map[string]float64
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	subscribeMu   sync.Mutex
}

/*
NewSignal creates the Level3 honesty calculator against the production Kraken
API so tests can replace only its connections, never its market mechanics.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	planner *strategy.Planner,
	ui chan []byte,
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		api:           api,
		previousTouch: make(map[string]float64),
		previousPrice: make(map[string]float64),
		planner:       planner,
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
	}

	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceToxicity)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	if signal.subscribers == nil {
		signal.subscribers = &sync.Map{}
	}

	return signalshared.Subscribe(
		&signal.subscribeMu,
		signal.subscribers,
		channel,
		subscription,
	)
}

/*
onTrade attributes public executions to the resting side they consumed while
retaining the same Level3 touch evidence used by book-only observations.
*/
func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-signal.subscriptions["thesis"].Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					thesis.AppendMeasurements(
						types.SourceToxicity,
						signal.Measure(thesis),
						types.Stamp{At: time.Now(), Entity: types.MarketTrade},
					)

					utils.Fanout(signal.subscribers, signal.Name(), thesis)
				}
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	_, trades, books := thesis.Market()
	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)

	var lastTime time.Time

	for _, trade := range trades {
		if lastTime.IsZero() || trade.Timestamp.After(lastTime) {
			lastTime = trade.Timestamp
		}

		found, ok := books.Load(trade.Symbol)

		if !ok || found == nil {
			continue
		}

		book := found.(*spotbook.Book)

		bid := book.BestBid()
		ask := book.BestAsk()

		if bid == nil || ask == nil {
			return nil
		}

		bidQuantity := bid.Quantity.Float64()
		askQuantity := ask.Quantity.Float64()
		touchQuantity := bidQuantity + askQuantity

		bidRetreat := signal.retreat(
			thesis,
			trade.Symbol,
			types.SideBuy,
			bid.Price.Float64(),
			bidQuantity,
		)

		askRetreat := signal.retreat(
			thesis,
			trade.Symbol,
			types.SideSell,
			ask.Price.Float64(),
			askQuantity,
		)

		buyFill := 0.0
		sellFill := 0.0

		if types.MeasurementSide(trade.Side) == types.SideBuy {
			buyFill = trade.Qty
		}

		if types.MeasurementSide(trade.Side) == types.SideSell {
			sellFill = trade.Qty
		}

		measurement := &types.Measurement{
			Source:   types.SourceToxicity,
			Symbol:   trade.Symbol,
			At:       trade.Timestamp,
			Maturity: float64(thesis.Tick),
			Validity: types.ObservationValidity(len(trades)),
			Scale: types.ScaleReference{
				Kind:    types.ScaleObservationWindow,
				From:    lastTime,
				Through: trade.Timestamp,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricTradeVolume, types.SideNone): {
					Raw:  trade.Qty,
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricFillVolume, types.SideBuy): {
					Raw:  buyFill,
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricFillVolume, types.SideSell): {
					Raw:  sellFill,
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricBestPrice, types.SideBuy): {
					Raw:  bid.Price.Float64(),
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricBestPrice, types.SideSell): {
					Raw:  ask.Price.Float64(),
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
					Raw:        bidQuantity,
					Normalized: types.NormalizeRatio(bidQuantity, touchQuantity),
					Unit:       types.UnitBaseCurrency,
				},
				types.MetricKey(types.MetricTouchQuantity, types.SideSell): {
					Raw:        askQuantity,
					Normalized: types.NormalizeRatio(askQuantity, touchQuantity),
					Unit:       types.UnitBaseCurrency,
				},
				types.MetricKey(types.MetricRetreatingQuantity, types.SideBuy): {
					Raw:        bidRetreat,
					Normalized: types.NormalizeRatio(bidRetreat, bidQuantity+bidRetreat),
					Unit:       types.UnitBaseCurrency,
				},
				types.MetricKey(types.MetricRetreatingQuantity, types.SideSell): {
					Raw:        askRetreat,
					Normalized: types.NormalizeRatio(askRetreat, askQuantity+askRetreat),
					Unit:       types.UnitBaseCurrency,
				},
				types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {
					Raw:  0,
					Unit: types.UnitBaseCurrency,
				},
				types.MetricKey(types.MetricCancelledQuantity, types.SideSell): {
					Raw:  0,
					Unit: types.UnitBaseCurrency,
				},
			},
		}

		measurements = append(measurements, measurement)

		if measurement.Symbol == types.Focus() {
			out = append(out, measurement)
		}
	}

	if len(out) > 0 {
		frame := datura.NewMap()
		frame["measurements"] = out
		utils.Publish(signal.ui, frame)
	}

	return measurements
}

/*
retreat reports visible touch liquidity that disappeared since the previous
observation for this symbol and side.
*/
func (signal *Signal) retreat(
	_ *types.Thesis,
	symbol string,
	side types.MeasurementSide,
	price float64,
	quantity float64,
) float64 {
	if signal.previousTouch == nil {
		signal.previousTouch = make(map[string]float64)
	}

	if signal.previousPrice == nil {
		signal.previousPrice = make(map[string]float64)
	}

	key := symbol + ":" + string(side)
	previousQuantity := signal.previousTouch[key]
	previousPrice := signal.previousPrice[key]
	signal.previousTouch[key] = quantity
	signal.previousPrice[key] = price

	if previousQuantity == 0 {
		return 0
	}

	if side == types.SideBuy && price < previousPrice {
		return previousQuantity
	}

	if side == types.SideSell && price > previousPrice {
		return previousQuantity
	}

	if price != previousPrice {
		return 0
	}

	if quantity >= previousQuantity {
		return 0
	}

	return previousQuantity - quantity
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
