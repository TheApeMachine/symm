package toxicity

import (
	"context"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
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
	thesis        *types.Thesis
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
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
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:  types.INITIALIZING,
		thesis:  planner.Thesis,
		ctx:     ctx,
		cancel:  cancel,
		api:     api,
		planner: planner,
		ui:      ui,
		subscriptions: map[string]*types.Subscription[any]{
			"trade": api.Subscribe("trade", types.NewSubscription[any]()),
		},
	}

	if signal.thesis != nil {
		signal.thesis.Causal.Store("signal:toxicity:touch", make(map[string]float64))
		signal.thesis.Causal.Store("signal:toxicity:price", make(map[string]float64))
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
			case message := <-signal.subscriptions["trade"].Channel:
				if trade, ok := message.(*kraken.Trade); ok {
					signal.onTrade(trade)
				}
			}
		}
	}()
}

func (signal *Signal) onTrade(trade *kraken.Trade) {
	signal.thesis.AppendMeasurements(
		types.SourceToxicity,
		signal.Calculate(nil, trade.Data),
		types.Stamp{At: time.Now(), Entity: types.MarketTrade},
	)
}

/*
Calculate converts public book rows into toxicity observations by sampling the
current authenticated Level3 book for each symbol at the public book timestamp.
*/
func (signal *Signal) Calculate(
	books []kraken.BookData,
	trades []kraken.TradeData,
) []*types.Measurement {
	measurements := make([]*types.Measurement, 0, len(books)+len(trades))

	for _, row := range books {
		measurement := signal.measure(row.Symbol, row.Timestamp.UTC(), 0, types.SideNone)

		if measurement == nil {
			continue
		}

		measurements = append(measurements, measurement)
	}

	for _, trade := range trades {
		measurement := signal.measure(
			trade.Symbol,
			trade.Timestamp.UTC(),
			trade.Price.Float64()*trade.Qty,
			signal.fillSide(trade.Side),
		)

		if measurement != nil {
			measurements = append(measurements, measurement)
		}
	}

	return measurements
}

/*
measure builds one toxicity row from the current Level3 book. Buy-side metrics
refer to resting bids and sell-side metrics refer to resting asks.
*/
func (signal *Signal) measure(
	symbol string,
	at time.Time,
	tradeVolume float64,
	fillSide types.MeasurementSide,
) *types.Measurement {
	var measurement *types.Measurement
	out := make([]types.Measurement, 0)
	book := signal.api.Book(symbol)

	if book == nil {
		return nil
	}

	bid := book.BestBid()
	ask := book.BestAsk()

	if bid == nil || ask == nil {
		return nil
	}

	bidQuantity := bid.Quantity.Float64()
	askQuantity := ask.Quantity.Float64()
	touchQuantity := bidQuantity + askQuantity
	bidRetreat := signal.retreat(symbol, types.SideBuy, bid.Price.Float64(), bidQuantity)
	askRetreat := signal.retreat(symbol, types.SideSell, ask.Price.Float64(), askQuantity)
	buyFill := 0.0
	sellFill := 0.0

	if fillSide == types.SideBuy {
		buyFill = tradeVolume
	}

	if fillSide == types.SideSell {
		sellFill = tradeVolume
	}

	measurement = &types.Measurement{
		Source:   types.SourceToxicity,
		Symbol:   symbol,
		At:       at,
		Maturity: signal.thesis.Tick,
		Validity: types.ObservationValidity(1),
		Scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    at,
			Through: at,
		},
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricTradeVolume, types.SideNone): {
				Raw:  tradeVolume,
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

	if measurement.Symbol == types.Focus() {
		out = append(out, *measurement)
	}

	if len(out) == 0 {
		return measurement
	}

	frame := datura.NewMap()
	frame["measurements"] = out
	utils.Publish(signal.ui, frame)

	return measurement
}

/*
retreat reports visible touch liquidity that disappeared since the previous
observation for this symbol and side.
*/
func (signal *Signal) retreat(
	symbol string,
	side types.MeasurementSide,
	price float64,
	quantity float64,
) float64 {
	key := symbol + ":" + string(side)
	foundTouch, _ := signal.thesis.Causal.Load("signal:toxicity:touch")
	foundPrice, _ := signal.thesis.Causal.Load("signal:toxicity:price")
	touch := foundTouch.(map[string]float64)
	prices := foundPrice.(map[string]float64)
	previousQuantity := touch[key]
	previousPrice := prices[key]
	touch[key] = quantity
	prices[key] = price

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
fillSide maps Kraken's aggressor side onto the resting book side that was hit.
*/
func (signal *Signal) fillSide(side string) types.MeasurementSide {
	if side == "buy" {
		return types.SideSell
	}

	if side == "sell" {
		return types.SideBuy
	}

	return types.SideNone
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
