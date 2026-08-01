package exhaust

import (
	"context"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	signalshared "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal tracks side-specific microstructure decay that can advise whether a long
or short position should exit. It emits numerical family scores, their fused
urgency, and the winning numerical family identifier for downstream logic.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	instrument    *broker.Instrument
	planner       *strategy.Planner
	sample        *algorithm.DecaySample
	decay         *equation.Decay
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	subscribeMu   sync.Mutex
}

/*
NewSignal constructs the market-wide exhaustion observer. Position inventory is
deliberately absent: the signal measures both hypothetical exit sides, leaving
the consumer to select the side matching its position.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
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
		instrument:    instrument,
		planner:       planner,
		sample:        algorithm.NewDecaySample(),
		decay:         equation.NewDecay(),
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
	return string(types.SourceExhaustion)
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

func (signal *Signal) run() {
	thesisSubscription := signal.subscriptions["thesis"]

	if thesisSubscription == nil {
		return
	}

	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-thesisSubscription.Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					thesis.AppendMeasurements(
						types.SourceExhaustion,
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

	books.Range(func(_, value any) bool {
		managed, ok := value.(*spotbook.Book)

		if !ok {
			return true
		}

		bookMeasurements, err := signal.measureManagedBook(managed)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"exhaust: failed to measure book",
				err,
			))

			return true
		}

		measurements = append(measurements, bookMeasurements...)
		return true
	})

	for _, trade := range trades {
		tradeMeasurements, err := signal.measureTrade(trade)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"exhaust: failed to measure trade",
				err,
			))
			continue
		}

		measurements = append(measurements, tradeMeasurements...)

		for _, measurement := range tradeMeasurements {
			if measurement.Symbol == types.Focus() {
				out = append(out, measurement)
			}
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", out))
	}

	return measurements
}

func (signal *Signal) measureManagedBook(
	managed *spotbook.Book,
) ([]*types.Measurement, error) {
	bids := make([]flow.BookLevel, 0)
	asks := make([]flow.BookLevel, 0)

	instrument, err := signal.instrument.Pair(managed.Name)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"exhaust: failed to resolve instrument",
			err,
		))
	}

	for _, bid := range managed.Bids.Levels {
		bids = append(bids, flow.BookLevel{
			Price:    bid.Price.Float64(),
			Quantity: bid.Quantity.Float64(),
			Ticks: bid.Price.Div(
				decimal.NewFromInt64(instrument.PriceIncrement.GetIncrement()),
			).Int64(),
		})
	}

	for _, ask := range managed.Asks.Levels {
		asks = append(asks, flow.BookLevel{
			Price:    ask.Price.Float64(),
			Quantity: ask.Quantity.Float64(),
			Ticks: ask.Price.Div(
				decimal.NewFromInt64(instrument.PriceIncrement.GetIncrement()),
			).Int64(),
		})
	}

	input, ready, maturity, err := signal.sample.MeasureBook(flow.BookInput{
		Symbol:   managed.Name,
		TickSize: instrument.TickSize.Float64(),
		Bids:     bids,
		Asks:     asks,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure book",
			err,
		))
	}

	if !ready {
		return []*types.Measurement{{
			Source:   types.SourceExhaustion,
			Symbol:   managed.Name,
			At:       managed.BestBid().Timestamp,
			Maturity: maturity,
			Validity: types.ObservationValidity(1),
		}}, nil
	}

	output, err := signal.decay.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure decay",
			err,
		))
	}

	return signal.frame(
		managed.Name,
		managed.BestBid().Timestamp,
		output,
		maturity,
	), nil
}

/*
measureTrade applies one trade event to the shared decay sample at its causal
position in the merged entity timeline.
*/
func (signal *Signal) measureTrade(
	row kraken.TradeData,
) ([]*types.Measurement, error) {
	if row.Symbol == "" || row.Timestamp.IsZero() || row.Price.Sign() <= 0 ||
		row.Qty <= 0 || row.Side != "buy" && row.Side != "sell" {
		return nil, nil
	}

	input, ready, maturity, err := signal.sample.MeasureTrade(flow.TradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price.Float64(),
		Quantity: row.Qty,
		Side:     flow.TradeSide(row.Side),
		At:       row.Timestamp,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure trade",
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := signal.decay.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"exhaust: failed to measure decay",
			err,
		))
	}

	return signal.frame(
		row.Symbol,
		row.Timestamp,
		output,
		maturity,
	), nil
}

/*
frame converts a decay calculator output into the shared Measurement shape, so
both the book-driven and trade-driven observation paths emit the same metric
set for a symbol.
*/
func (signal *Signal) frame(
	symbol string,
	at time.Time,
	output equation.DecayOutput,
	maturity float64,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	measurement := &types.Measurement{
		Source:       types.SourceExhaustion,
		Symbol:       symbol,
		At:           at,
		ObservedFrom: at,
		Maturity:     maturity,
		Validity:     validity,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricMechanical, types.SideBuy): {Raw: output.Long.Mechanical,
				Normalized: types.NormalizeFinite(output.Long.Mechanical),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricThermal, types.SideBuy): {
				Raw:        output.Long.Thermal,
				Normalized: types.NormalizeFinite(output.Long.Thermal),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricFragile, types.SideBuy): {
				Raw:        output.Long.Fragile,
				Normalized: types.NormalizeFinite(output.Long.Fragile),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricReversal, types.SideBuy): {
				Raw:        output.Long.Reversal,
				Normalized: types.NormalizeFinite(output.Long.Reversal),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricUrgency, types.SideBuy): {
				Raw:        output.Long.Urgency,
				Normalized: types.NormalizeFinite(output.Long.Urgency),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStrength, types.SideBuy): {
				Raw:        output.Long.Strength,
				Normalized: types.NormalizeFinite(output.Long.Strength),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricValue, types.SideBuy): {
				Raw:        output.Long.Value,
				Normalized: types.NormalizeFinite(output.Long.Value),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricCategory, types.SideBuy): {
				Raw:        output.Long.Category,
				Normalized: types.NormalizeFinite(output.Long.Category),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricMechanical, types.SideSell): {
				Raw:        output.Short.Mechanical,
				Normalized: types.NormalizeFinite(output.Short.Mechanical),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricThermal, types.SideSell): {
				Raw:        output.Short.Thermal,
				Normalized: types.NormalizeFinite(output.Short.Thermal),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricFragile, types.SideSell): {
				Raw:        output.Short.Fragile,
				Normalized: types.NormalizeFinite(output.Short.Fragile),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricReversal, types.SideSell): {
				Raw:        output.Short.Reversal,
				Normalized: types.NormalizeFinite(output.Short.Reversal),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricUrgency, types.SideSell): {
				Raw:        output.Short.Urgency,
				Normalized: types.NormalizeFinite(output.Short.Urgency),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStrength, types.SideSell): {
				Raw:        output.Short.Strength,
				Normalized: types.NormalizeFinite(output.Short.Strength),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricValue, types.SideSell): {
				Raw:        output.Short.Value,
				Normalized: types.NormalizeFinite(output.Short.Value),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricCategory, types.SideSell): {
				Raw:        output.Short.Category,
				Normalized: types.NormalizeFinite(output.Short.Category),
				Unit:       types.UnitDimensionless,
			},
		},
	}

	return []*types.Measurement{measurement}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
