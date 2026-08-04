package pumpdump

import (
	"context"
	"math"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
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
	subscribeMu   sync.Mutex
	lastTrade     map[string]tradeCursor
}

type tradeCursor struct {
	at  time.Time
	ids map[int64]struct{}
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
		lastTrade:     make(map[string]tradeCursor),
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
	return utils.Subscribe(
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-signal.subscriptions["thesis"].Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					measurements := signal.Measure(thesis)

					if len(measurements) > 0 {
						thesis.Measurements.Store(
							types.SourcePumpDump,
							measurements,
						)

						thesis.Readiness.PumpDump = true
						utils.Fanout(signal.subscribers, signal.Name(), thesis)
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
	out := make([]*types.Measurement, 0)

	trades := thesis.MarketTrades()
	books := thesis.Books

	if len(trades) == 0 {
		return measurements
	}

	for _, trade := range trades {
		if !validTrade(trade) || signal.seenTrade(trade) {
			continue
		}

		found, ok := books.Load(trade.Symbol)

		if !ok || found == nil {
			continue
		}

		book, ok := found.(*spotbook.Book)

		if !ok || book == nil {
			continue
		}

		ask, bid := book.BestAsk(), book.BestBid()

		if ask == nil || bid == nil || ask.Price == nil || bid.Price == nil {
			continue
		}

		if ask.Timestamp.After(trade.Timestamp) || bid.Timestamp.After(trade.Timestamp) {
			continue
		}

		askPrice := ask.Price.Float64()
		bidPrice := bid.Price.Float64()
		mid := (askPrice + bidPrice) / 2

		if bidPrice <= 0 || askPrice <= bidPrice || math.IsNaN(mid) || math.IsInf(mid, 0) {
			continue
		}

		output, ready, maturity, err := signal.algo.Measure(equation.IgnitionInput{
			At:     trade.Timestamp,
			Symbol: trade.Symbol,
			Last:   trade.Price.Float64(),
			Volume: trade.Qty,
			Ask:    askPrice,
			Bid:    bidPrice,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"pumpdump: failed to measure ignition",
				err,
			))

			continue
		}

		signal.commitTrade(trade)

		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}

		if !ready {
			validity.State = types.ValidityProvisional
			validity.Reason = "ignition baseline not ready"
		}

		measurement := &types.Measurement{
			Source:   types.SourcePumpDump,
			Symbol:   trade.Symbol,
			At:       trade.Timestamp,
			Maturity: maturity,
			Validity: validity,
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
					Normalized: types.NormalizeRatio(output.Spread, mid),
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
				types.MetricKey(types.MetricPrecursor, types.SideBuy): {
					Raw:        output.Buy.Precursor,
					Normalized: types.NormalizeFinite(output.Buy.Precursor),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricCompression, types.SideBuy): {
					Raw:        output.Buy.Compression,
					Normalized: types.NormalizeFinite(output.Buy.Compression),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricIgnition, types.SideBuy): {
					Raw:        output.Buy.Ignition,
					Normalized: types.NormalizeFinite(output.Buy.Ignition),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricTrend, types.SideBuy): {
					Raw:        output.Buy.Trend,
					Normalized: types.NormalizeFinite(output.Buy.Trend),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricExhaustion, types.SideBuy): {
					Raw:        output.Buy.Exhaustion,
					Normalized: types.NormalizeFinite(output.Buy.Exhaustion),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStrength, types.SideBuy): {
					Raw:        output.Buy.Strength,
					Normalized: types.NormalizeFinite(output.Buy.Strength),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricPrecursor, types.SideSell): {
					Raw:        output.Sell.Precursor,
					Normalized: types.NormalizeFinite(output.Sell.Precursor),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricCompression, types.SideSell): {
					Raw:        output.Sell.Compression,
					Normalized: types.NormalizeFinite(output.Sell.Compression),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricIgnition, types.SideSell): {
					Raw:        output.Sell.Ignition,
					Normalized: types.NormalizeFinite(output.Sell.Ignition),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricTrend, types.SideSell): {
					Raw:        output.Sell.Trend,
					Normalized: types.NormalizeFinite(output.Sell.Trend),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricExhaustion, types.SideSell): {
					Raw:        output.Sell.Exhaustion,
					Normalized: types.NormalizeFinite(output.Sell.Exhaustion),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStrength, types.SideSell): {
					Raw:        output.Sell.Strength,
					Normalized: types.NormalizeFinite(output.Sell.Strength),
					Unit:       types.UnitDimensionless,
				},
			},
		}

		measurements = append(measurements, measurement)

		if measurement.Symbol == types.Focus() {
			out = append(out, measurement)
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", out))
	}

	return measurements
}

func validTrade(row kraken.TradeData) bool {
	price := row.Price.Float64()

	return row.Symbol != "" && !row.Timestamp.IsZero() && price > 0 && row.Qty > 0 &&
		!math.IsNaN(price) && !math.IsInf(price, 0) && !math.IsNaN(row.Qty) &&
		!math.IsInf(row.Qty, 0) && (row.Side == "buy" || row.Side == "sell")
}

func (signal *Signal) seenTrade(row kraken.TradeData) bool {
	previous := signal.lastTrade[row.Symbol]

	if row.Timestamp.Before(previous.at) {
		return true
	}

	if row.Timestamp.After(previous.at) {
		return false
	}

	_, seen := previous.ids[row.TradeID]

	return seen
}

func (signal *Signal) commitTrade(row kraken.TradeData) {
	previous := signal.lastTrade[row.Symbol]

	if row.Timestamp.After(previous.at) {
		previous = tradeCursor{at: row.Timestamp, ids: make(map[int64]struct{})}
	}

	if previous.ids == nil {
		previous.ids = make(map[int64]struct{})
	}

	previous.ids[row.TradeID] = struct{}{}
	signal.lastTrade[row.Symbol] = previous
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
