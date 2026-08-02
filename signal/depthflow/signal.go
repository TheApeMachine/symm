package depthflow

import (
	"context"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Categories belong in logic; this
signal emits numerical scores only.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	instrument    *broker.Instrument
	planner       *strategy.Planner
	sample        *flow.Sample
	bookflow      *equation.Bookflow
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	subscribeMu   sync.Mutex
}

/*
NewSignal creates depth-flow state shared by the causally ordered book and
trade observations in each central market cut.
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
	sample, err := flow.NewSample(viper.GetViper().GetInt("signals.depthflow.sampleSize"))

	if err != nil {
		cancel()
		errnie.Error(errnie.Err(
			errnie.Validation,
			"depthflow: failed to create flow sample",
			err,
		))
		return nil
	}

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		api:           api,
		instrument:    instrument,
		planner:       planner,
		sample:        sample,
		bookflow:      equation.NewBookflow(),
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
	return string(types.SourceDepthFlow)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return utils.Subscribe(
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
						types.SourceDepthFlow,
						signal.Measure(thesis),
						types.Stamp{
							At:     time.Now(),
							Entity: types.MarketTrade,
							Source: types.SourceDepthFlow,
						},
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
				"depthflow: failed to measure book",
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
				"depthflow: failed to measure trade",
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
			"depthflow: failed to resolve instrument",
			err,
		))
	}

	for bid := managed.Bids.High; bid != nil; bid = bid.Lower {
		bids = append(bids, flow.BookLevel{
			Price:    bid.Price.Float64(),
			Quantity: bid.Quantity.Float64(),
			Ticks: decimal.ExactDiv(
				bid.Price,
				&instrument.PriceIncrement,
			).Int64(),
		})
	}

	for ask := managed.Asks.Low; ask != nil; ask = ask.Higher {
		asks = append(asks, flow.BookLevel{
			Price:    ask.Price.Float64(),
			Quantity: ask.Quantity.Float64(),
			Ticks: decimal.ExactDiv(
				ask.Price,
				&instrument.PriceIncrement,
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
			"depthflow: failed to measure book",
			err,
		))
	}

	if managed.BestBid() == nil || managed.BestAsk() == nil {
		return nil, nil
	}

	if !ready {
		return []*types.Measurement{{
			Source:   types.SourceDepthFlow,
			Symbol:   managed.Name,
			At:       managed.BestBid().Timestamp,
			Maturity: maturity,
			Validity: types.ObservationValidity(1),
		}}, nil
	}

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure bookflow",
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
measureTrade applies one trade event to the shared flow sample at its causal
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
			"depthflow: failed to measure trade",
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := signal.bookflow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"depthflow: failed to measure bookflow",
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
frame converts a bookflow calculator output into one source×symbol row so both
the book-driven and trade-driven observation paths emit the same metric set.
*/
func (signal *Signal) frame(
	symbol string, at time.Time,
	output equation.BookflowOutput,
	maturity float64,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	measurement := &types.Measurement{
		Source:       types.SourceDepthFlow,
		Symbol:       symbol,
		At:           at,
		ObservedFrom: at,
		Maturity:     maturity,
		Validity:     validity,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricLoadedScore, types.SideNone): {Raw: output.LoadedScore,
				Normalized: types.NormalizeFinite(output.LoadedScore),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSpoofScore, types.SideNone): {
				Raw:        output.SpoofScore,
				Normalized: types.NormalizeFinite(output.SpoofScore),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricThinScore, types.SideNone): {
				Raw:        output.ThinScore,
				Normalized: types.NormalizeFinite(output.ThinScore),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricNeutralScore, types.SideNone): {
				Raw:        output.NeutralScore,
				Normalized: types.NormalizeFinite(output.NeutralScore),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStrength, types.SideNone): {
				Raw:        output.Strength,
				Normalized: types.NormalizeFinite(output.Strength),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricValue, types.SideNone): {
				Raw:        output.Value,
				Normalized: types.NormalizeFinite(output.Value),
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
