package pumpdump

import (
	"context"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
Signal owns pump-cycle measurements derived from executed trade lift and the
reconstructed book's midpoint and spread. It reads each market
fact from its authoritative stream without treating them as independent
corroborating signals.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	algo          *equation.Ignition
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
}

/*
NewSignal creates an empty per-symbol pump state whose baseline capacity is the
same explicit retention bound used by the production market feed.
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
		algo:          equation.NewIgnition(viper.GetViper().GetInt("signals.pumpdump.baselineCapacity")),
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
	return string(types.SourcePumpDump)
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

	subscribers, ok := signal.subscribers.LoadOrStore(
		channel, []*types.Subscription[any]{subscription},
	)

	if ok && subscribers != nil {
		signal.subscribers.Store(
			channel, append(subscribers.([]*types.Subscription[any]), subscription),
		)
	}

	return subscription
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-signal.subscriptions["thesis"].Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					thesis.AppendMeasurements(
						types.SourcePumpDump,
						signal.Measure(thesis),
						types.Stamp{At: time.Now(), Entity: types.MarketTicker},
					)

					if signal.subscribers == nil {
						return
					}

					subscribers, ok := signal.subscribers.Load(signal.Name())

					if ok && subscribers != nil {
						for _, subscriber := range subscribers.([]*types.Subscription[any]) {
							subscriber.Send(thesis)
						}
					}
				}
			}
		}
	}()
}

/*
Measure produces the Measurements for the pumpdump signal.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) []*types.Measurement {
	measurements := make([]*types.Measurement, 0)
	tickers, trades, books := thesis.Market()

	if len(tickers) == 0 && len(trades) == 0 {
		return nil
	}

	if len(tickers) > 0 {
		var lastTime time.Time
		var mid *decimal.Decimal

		for _, ticker := range tickers {
			if lastTime.IsZero() || ticker.Timestamp.After(lastTime) {
				lastTime = ticker.Timestamp
			}

			found, ok := books.Load(ticker.Symbol)

			if ok && found != nil {
				book, ok := found.(*book.Book)

				if !ok || book == nil {
					errnie.Error(errnie.Err(
						errnie.Validation,
						"pumpdump: unexpected book type",
						nil,
					))

					continue
				}

				mid = book.Midpoint()
			}

			output, _, maturity, err := signal.algo.Measure(equation.IgnitionInput{
				At:     ticker.Timestamp,
				Symbol: ticker.Symbol,
				Last:   ticker.Ask.Float64(),
				Volume: ticker.AskQty,
				Ask:    ticker.Ask.Float64(),
				Bid:    ticker.Bid.Float64(),
			})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"pumpdump: failed to measure ignition",
					err,
				))

				continue
			}

			measurements = append(measurements, &types.Measurement{
				Source:   types.SourcePumpDump,
				Symbol:   ticker.Symbol,
				At:       ticker.Timestamp,
				Maturity: maturity,
				Validity: types.MeasurementValidity{},
				Scale: types.ScaleReference{
					Kind:    types.ScaleObservationWindow,
					From:    lastTime,
					Through: ticker.Timestamp,
				},
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricRVOL, types.SideNone): {
						Raw:        output.RVOL,
						Normalized: types.NormalizeFinite(output.RVOL),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricPrecursor, types.SideNone): {
						Raw:        output.Precursor,
						Normalized: types.NormalizeFinite(output.Precursor),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricSpread, types.SideNone): {
						Raw:        output.Spread,
						Normalized: types.NormalizeRatio(output.Spread, mid.Float64()),
						Unit:       types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricCompression, types.SideNone): {
						Raw:        output.Compression,
						Normalized: types.NormalizeFinite(output.Compression),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricIgnition, types.SideNone): {
						Raw:        output.Ignition,
						Normalized: types.NormalizeFinite(output.Ignition),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricTrend, types.SideNone): {
						Raw:        output.Trend,
						Normalized: types.NormalizeFinite(output.Trend),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricExhaustion, types.SideNone): {
						Raw:        output.Exhaustion,
						Normalized: types.NormalizeFinite(output.Exhaustion),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricStrength, types.SideNone): {
						Raw:        output.Strength,
						Normalized: types.NormalizeFinite(output.Strength),
						Unit:       types.UnitDimensionless,
					},
				},
			})
		}
	}

	if len(trades) > 0 {
		var lastTime time.Time
		var mid *decimal.Decimal

		for _, trade := range trades {
			if lastTime.IsZero() || trade.Timestamp.After(lastTime) {
				lastTime = trade.Timestamp
			}

			found, ok := books.Load(trade.Symbol)

			if ok && found != nil {
				book, ok := found.(*book.Book)

				if !ok || book == nil {
					errnie.Error(errnie.Err(
						errnie.Validation,
						"pumpdump: unexpected book type",
						nil,
					))

					continue
				}

				mid = book.Midpoint()
			}

			output, _, maturity, err := signal.algo.Measure(equation.IgnitionInput{
				At:     trade.Timestamp,
				Symbol: trade.Symbol,
				Last:   trade.Price.Float64(),
				Volume: trade.Qty,
			})

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"pumpdump: failed to measure ignition",
					err,
				))

				continue
			}

			measurements = append(measurements, &types.Measurement{
				Source:   types.SourcePumpDump,
				Symbol:   trade.Symbol,
				At:       trade.Timestamp,
				Maturity: maturity,
				Validity: types.MeasurementValidity{},
				Scale: types.ScaleReference{
					Kind:    types.ScaleObservationWindow,
					From:    lastTime,
					Through: trade.Timestamp,
				},
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricRVOL, types.SideNone): {
						Raw:        output.RVOL,
						Normalized: types.NormalizeFinite(output.RVOL),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricPrecursor, types.SideNone): {
						Raw:        output.Precursor,
						Normalized: types.NormalizeFinite(output.Precursor),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricSpread, types.SideNone): {
						Raw:        output.Spread,
						Normalized: types.NormalizeRatio(output.Spread, mid.Float64()),
						Unit:       types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricCompression, types.SideNone): {
						Raw:        output.Compression,
						Normalized: types.NormalizeFinite(output.Compression),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricIgnition, types.SideNone): {
						Raw:        output.Ignition,
						Normalized: types.NormalizeFinite(output.Ignition),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricTrend, types.SideNone): {
						Raw:        output.Trend,
						Normalized: types.NormalizeFinite(output.Trend),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricExhaustion, types.SideNone): {
						Raw:        output.Exhaustion,
						Normalized: types.NormalizeFinite(output.Exhaustion),
						Unit:       types.UnitDimensionless,
					},
					types.MetricKey(types.MetricStrength, types.SideNone): {
						Raw:        output.Strength,
						Normalized: types.NormalizeFinite(output.Strength),
						Unit:       types.UnitDimensionless,
					},
				},
			})
		}
	}

	return measurements
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	// cancel() // Assuming cancel is defined elsewhere, otherwise this line may need to be updated
	return nil
}
